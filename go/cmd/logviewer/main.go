package main

import (
	"bufio"
	"embed"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

//go:embed index.html
var staticFiles embed.FS

var (
	logDir        string
	port          int
	validFileName = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

var (
	ErrMissingFilename       = errors.New("filename parameter missing")
	ErrInvalidFilenameFormat = errors.New("invalid filename format")
	ErrInvalidFilenameChars  = errors.New("invalid characters in filename")
	ErrPathTraversalDetected = errors.New("path traversal detected")
	ErrLogFileNotFound       = errors.New("log file not found")
	ErrLogFileAccessDenied   = errors.New("log file access denied")
	ErrLogFileOpenFailed     = errors.New("failed to open log file")
	ErrLogFileScanFailed     = errors.New("failed to scan log file")
)

const (
	maxLogLines    = 2000
	maxLogLineSize = 10 * 1024 * 1024 // 10 MB
)

func init() {
	flag.StringVar(&logDir, "log-dir", ".", "Directory containing log files")
	flag.IntVar(&port, "port", 8080, "Port to run the server on")
}

func main() {
	flag.Parse()

	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/api/logs", handleLogs)
	http.HandleFunc("/api/files", handleFiles)

	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("Starting Log Viewer at http://localhost%s\n", addr)
	fmt.Printf("Watching log directory: %s\n", logDir)

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	data, err := staticFiles.ReadFile("index.html")
	if err != nil {
		http.Error(w, "Internal Server Error", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write(data)
}

func sanitizeFileName(filename string) (string, error) {
	if filename == "" {
		return "", ErrMissingFilename
	}

	cleanName := filepath.Base(filename)
	if cleanName != filename || cleanName == "." || cleanName == ".." {
		return "", ErrInvalidFilenameFormat
	}

	if !validFileName.MatchString(cleanName) {
		return "", ErrInvalidFilenameChars
	}

	fullPath := filepath.Join(logDir, cleanName)

	realPath, err := filepath.EvalSymlinks(fullPath)
	if err != nil {
		return "", ErrLogFileNotFound
	}

	absLogDir, _ := filepath.Abs(logDir)
	if !strings.HasPrefix(realPath, absLogDir+string(os.PathSeparator)) {
		return "", ErrPathTraversalDetected
	}

	return realPath, nil
}

func streamFilteredLogLines(w http.ResponseWriter, file *os.File, queryLower string) error {
	var lines []string
	scanner := bufio.NewScanner(file)

	// Create a large buffer to handle long log lines if necessary
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, maxLogLineSize)

	for scanner.Scan() {
		line := scanner.Text()
		if queryLower == "" || strings.Contains(strings.ToLower(line), queryLower) {
			if len(lines) >= maxLogLines {
				lines = lines[1:] // Drop the oldest line
			}
			lines = append(lines, line)
		}
	}

	if err := scanner.Err(); err != nil {
		// If we haven't found any lines, treat this as a failure
		if len(lines) == 0 {
			return fmt.Errorf("scan failed: %w", err)
		}
		// Otherwise, log error and notify client of truncation, but serve what we have
		log.Printf("Error scanning file (partial content): %v", err)
		w.Header().Set("X-Log-Truncated", "true")
		w.Header().Set("X-Log-Error", err.Error())
	}

	w.Header().Set("Content-Type", "text/plain")
	for _, line := range lines {
		w.Write([]byte(line + "\n"))
	}
	return nil
}

func handleLogs(w http.ResponseWriter, r *http.Request) {
	filename := r.URL.Query().Get("file")

	logFilePath, err := sanitizeFileName(filename)
	if err != nil {
		switch err {
		case ErrMissingFilename:
			http.Error(w, "Filename parameter is required", 400)
		case ErrInvalidFilenameFormat, ErrInvalidFilenameChars:
			http.Error(w, "Invalid filename format", 400)
		case ErrPathTraversalDetected:
			http.Error(w, "Path traversal detected", 400)
		case ErrLogFileNotFound:
			http.Error(w, "Log file not found", 404)
		default:
			http.Error(w, "Internal Server Error", 500)
		}
		return
	}

	// limit := 1000 // Hardcoded limit for now
	file, err := os.Open(logFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "Log file not found", 404)
			return
		}
		http.Error(w, fmt.Sprintf("Error opening log file: %v", err), 500)
		return
	}
	defer file.Close()

	// Simple implementation: Read all, filter, return last N
	// For production, this should use seek or 'tail' logic.
	query := r.URL.Query().Get("q")
	queryLower := strings.ToLower(query)

	if err := streamFilteredLogLines(w, file, queryLower); err != nil {
		http.Error(w, fmt.Sprintf("Error processing log file: %v", err), 500)
		return
	}
}

func handleFiles(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error reading log directory: %v", err), 500)
		return
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".log") {
			files = append(files, entry.Name())
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("["))
	for i, f := range files {
		if i > 0 {
			w.Write([]byte(","))
		}
		w.Write([]byte(fmt.Sprintf("\"%s\"", f)))
	}
	w.Write([]byte("]"))
}
