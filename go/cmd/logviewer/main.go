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

	cleanName := filepath.Base(filename)
	if cleanName != filename || cleanName == "." || cleanName == ".." {
		http.Error(w, "Invalid file name format", 400)
		return
	}

	// 2. 白名单正则校验 (既防攻击也防特殊字符)
	if !validFileName.MatchString(cleanName) {
		http.Error(w, "Invalid characters in file name", 400)
		return
	}

	// 3. 构造完整路径
	fullPath := filepath.Join(logDir, cleanName)

	// 4. 物理路径校验 (防御符号链接穿越)
	// 注意：如果文件不存在，EvalSymlinks 会报错。
	// 如果是读取现有日志，这种写法很完美。
	realPath, err := filepath.EvalSymlinks(fullPath)
	if err != nil {
		// 如果文件不存在，根据业务逻辑决定是报404还是403
		http.Error(w, "File not found or access denied", 404)
		return
	}

	// 确保 logDir 也是绝对路径且经过清洗
	absLogDir, _ := filepath.Abs(logDir)

	// 最终前缀检查
	if !strings.HasPrefix(realPath, absLogDir+string(os.PathSeparator)) {
		http.Error(w, "Path escape detected", 403)
		return
	}
	logFilePath := realPath

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
