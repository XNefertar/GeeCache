package main

import (
	"bufio"
	"embed"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

//go:embed index.html
var staticFiles embed.FS

var (
	logDir string
	port   int
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

func handleLogs(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	filename := r.URL.Query().Get("file")
	if filename == "" {
		http.Error(w, "File parameter missing", 400)
		return
	}
	logFilePath := filepath.Join(logDir, filename)

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

	var lines []string
	scanner := bufio.NewScanner(file)

	// Create a large buffer to handle long log lines if necessary
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if query != "" {
			if strings.Contains(strings.ToLower(line), strings.ToLower(query)) {
				lines = append(lines, line)
			}
		} else {
			lines = append(lines, line)
		}
	}

	if err := scanner.Err(); err != nil {
		// Log error but continue serving what we have
		log.Printf("Error scanning file: %v", err)
	}

	// Helper to get last N lines
	maxLines := 2000
	start := 0
	if len(lines) > maxLines {
		start = len(lines) - maxLines
	}

	w.Header().Set("Content-Type", "text/plain")
	for i := start; i < len(lines); i++ {
		w.Write([]byte(lines[i] + "\n"))
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
