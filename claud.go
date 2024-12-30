package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	host            = "127.0.0.1"
	STATUS_FILE_DIR = "./logs/sftp-download-status" // 状态文件存储目录
)

type TaskStatus struct {
	TraceID     string    `json:"trace_id"`
	Status      string    `json:"status"` // pending, downloading, completed, failed
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time,omitempty"`
	Error       string    `json:"error,omitempty"`
	Source      string    `json:"source"`
	LogPath     string    `json:"logPath"`
	Destination string    `json:"destination"`
}

func saveTaskStatus(task *TaskStatus) error {
	if err := os.MkdirAll(STATUS_FILE_DIR, 0755); err != nil {
		return err
	}

	statusFile := filepath.Join(STATUS_FILE_DIR, task.TraceID+".json")
	data, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(statusFile, data, 0644)
}

func getConfig(username, password, certPath, traceID string, task *TaskStatus) (string, error) {
	// 创建临时的 rclone 配置文件
	configPath := filepath.Join(os.TempDir(), fmt.Sprintf("rclone_%s.conf", traceID))

	config := fmt.Sprintf(`
[sftp]
type = sftp
host = %s
user = %s
port = 22
`, host, username)

	if password != "" {
		cmd := exec.Command("rclone", "obscure", password)
		obscuredPass, err := cmd.Output()
		if err != nil {
			task.Status = "failed"
			task.Error = fmt.Sprintf("Failed to obscure password: %v", err)
			task.EndTime = time.Now()
			saveTaskStatus(task)
			return "", err
		}
		// 移除输出中的换行符
		config += fmt.Sprintf("pass = %s\n", strings.TrimSpace(string(obscuredPass)))
	}

	if certPath != "" {
		config += fmt.Sprintf("key_file = %s\n", certPath)
	}

	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		task.Status = "write config file failed"
		task.Error = err.Error()
		task.EndTime = time.Now()
		saveTaskStatus(task)
		return "", err
	}
	return configPath, nil
}

// 1. 配置文件不存在

func startDownload(username, password, certPath, remotePath, localPath, logPath, traceID string, wg *sync.WaitGroup) {
	defer wg.Done()
	task := &TaskStatus{
		TraceID:     traceID,
		Status:      "downloading",
		StartTime:   time.Now(),
		Source:      remotePath,
		Destination: localPath,
		LogPath:     logPath,
	}

	if err := os.MkdirAll(logPath, 0755); err != nil {
		task.Status = "failed"
		task.Error = fmt.Sprintf("Failed to create log directory: %v", err)
		task.EndTime = time.Now()
		saveTaskStatus(task)
		return
	}

	logFilePath := filepath.Join(logPath, fmt.Sprintf("%s.log", traceID))

	configPath, err := getConfig(username, password, certPath, traceID, task)
	defer os.Remove(configPath)
	if err != nil {
		return
	}

	cmd := exec.Command("rclone",
		"copy",
		fmt.Sprintf("sftp:%s", remotePath),
		localPath,
		"--config", configPath,
		"--log-file", logFilePath,
		"--log-level", "INFO",
		// "--delete-excluded", "false",
	)
	// cmd.Stdout = os.Stdout
	// cmd.Stderr = os.Stderr
	// // 合并任务状态和日志摘要
	// downloadLogPath := filepath.Join(logPath, fmt.Sprintf("download_%s.log", traceID))
	// logFile, err := os.Create(downloadLogPath)
	// if err != nil {
	// 	task.Status = "failed"
	// 	task.Error = fmt.Sprintf("无法创建下载日志文件: %v", err)
	// 	task.EndTime = time.Now()
	// 	return
	// }
	// defer logFile.Close()

	// 将命令输出重定向到日志文件
	// cmd.Stdout = logFile
	// cmd.Stderr = logFile

	saveTaskStatus(task)

	if err := cmd.Run(); err != nil {
		task.Status = "failed"
		task.Error = err.Error()
		task.EndTime = time.Now()
		saveTaskStatus(task)
		return
	}
	task.Status = "completed"
	task.EndTime = time.Now()

	logSummary(task)
	saveTaskStatus(task)

}

func main() {
	downloadCmd := flag.NewFlagSet("download", flag.ExitOnError)
	username := downloadCmd.String("user", "", "SFTP username")
	password := downloadCmd.String("pass", "", "SFTP password")
	certPath := downloadCmd.String("key", "", "Path to SSH private key")
	localPath := downloadCmd.String("local", "", "Local destination path")
	logPath := downloadCmd.String("log", "./logs", "Directory to store logs")
	fileList := downloadCmd.String("file-list", "", "File containing a list of remote file paths")
	// flag.Parse()

	statusCmd := flag.NewFlagSet("status", flag.ExitOnError)
	traceID := statusCmd.String("id", "", "Trace ID of the download task")

	switch os.Args[1] {
	case "download":
		downloadCmd.Parse(os.Args[2:])

		// fmt.Println(os.Args)
		if *username == "" {
			*username = os.Getenv("SFTP_USERNAME")
			if *username == "" {
				fmt.Println("user is not set")
				flag.Usage()
				// os.Exit(1)
			}
		}

		if *password == "" && *certPath == "" {
			*password = os.Getenv("SFTP_PASSWORD")
			*certPath = os.Getenv("SFTP_CERT_PATH")

			if *password == "" && *certPath == "" {
				fmt.Println("pass or cert path is required")
				flag.Usage()
				os.Exit(1)
			}
		}
		if *localPath == "" {
			*localPath = os.Getenv("SFTP_LOCAL_PATH")
			if *localPath == "" {
				fmt.Println("local path is not set")
				flag.Usage()
				os.Exit(1)
			}
		}

		// 设置环境变量以防止删除本地文件
		os.Setenv("RCLONE_SFTP_NO_REMOVE_DELETED_FILES", "true")

		if *fileList == "" {
			*fileList = os.Getenv("SFTP_FILE_LIST")
			if *fileList == "" {
				fmt.Println("file list is not set")
				flag.Usage()
				os.Exit(1)
			}
		}

		file, err := os.Open(*fileList)
		if err != nil {
			log.Fatalf("Failed to open file list: %v", err)
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		var remotePaths []string
		for scanner.Scan() {
			remotePaths = append(remotePaths, scanner.Text())
		}

		if err := scanner.Err(); err != nil {
			log.Fatalf("Error reading file list: %v", err)
		}

		if len(remotePaths) == 0 {
			log.Fatalf("No remote paths found in the file list")
		}

		var wg sync.WaitGroup
		maxConcurrent := 3
		sem := make(chan struct{}, maxConcurrent)

		for _, remotePath := range remotePaths {
			sem <- struct{}{}
			wg.Add(1)
			traceID := uuid.New().String()
			go func(remotePath string) {
				defer func() { <-sem }()
				startDownload(*username, *password, *certPath, remotePath, *localPath, *logPath, traceID, &wg)
			}(remotePath)
		}

		wg.Wait()
		fmt.Println("All downloads completed.")
	case "status":
		statusCmd.Parse(os.Args[2:])

		if *traceID == "" {
			fmt.Println("Please provide trace ID")
			statusCmd.PrintDefaults()
			os.Exit(1)
		}

		// 读取任务状态
		task, err := loadTaskStatus(*traceID)
		if err != nil {
			fmt.Printf("Failed to get task status: %v\n", err)
			os.Exit(1)
		}

		// 输出状态信息
		statusJSON, _ := json.MarshalIndent(task, "", "  ")
		fmt.Println(string(statusJSON))

	default:
		fmt.Println("expected 'download' or 'status' subcommands")
		os.Exit(1)
	}
}

func loadTaskStatus(traceID string) (*TaskStatus, error) {
	statusFile := filepath.Join(STATUS_FILE_DIR, traceID+".json")
	data, err := os.ReadFile(statusFile)
	if err != nil {
		return nil, err
	}

	var task TaskStatus
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, err
	}

	return &task, nil
}

func logSummary(task *TaskStatus) {
	localPath := task.Destination
	remotePath := task.Source
	traceID := task.TraceID
	// 生成下载完成的概要日志
	logPath := filepath.Join(task.LogPath, fmt.Sprintf("download_%s.log", traceID))

	// 收集已下载的文件信息
	var fileInfos []string
	var totalSize int64

	err := filepath.Walk(localPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		// 计算相对路径
		relPath, err := filepath.Rel(localPath, path)
		if err != nil {
			return err
		}

		// 格式化文件大小
		size := info.Size()
		totalSize += size
		var sizeStr string
		switch {
		case size > 1024*1024*1024:
			sizeStr = fmt.Sprintf("%.2f GB", float64(size)/(1024*1024*1024))
		case size > 1024*1024:
			sizeStr = fmt.Sprintf("%.2f MB", float64(size)/(1024*1024))
		case size > 1024:
			sizeStr = fmt.Sprintf("%.2f KB", float64(size)/1024)
		default:
			sizeStr = fmt.Sprintf("%d B", size)
		}

		// 添加文件信息
		if len(relPath) > 80 {
			relPath = relPath[:80]
		}
		fileInfos = append(fileInfos, fmt.Sprintf("- %-80s  Size: %-20s  Modified: %s",
			relPath,
			sizeStr,
			info.ModTime().Format(time.RFC3339),
		))
		return nil
	})

	if err != nil {
		log.Printf("Error collecting file information: %v", err)
	}

	// 格式化总大小
	var totalSizeStr string
	switch {
	case totalSize > 1024*1024*1024:
		totalSizeStr = fmt.Sprintf("%.2f GB", float64(totalSize)/(1024*1024*1024))
	case totalSize > 1024*1024:
		totalSizeStr = fmt.Sprintf("%.2f MB", float64(totalSize)/(1024*1024))
	case totalSize > 1024:
		totalSizeStr = fmt.Sprintf("%.2f KB", float64(totalSize)/1024)
	default:
		totalSizeStr = fmt.Sprintf("%d B", totalSize)
	}

	// 生成日志内容
	var logContent strings.Builder
	logContent.WriteString(fmt.Sprintf("Download Summary\n===============\n\n"))
	logContent.WriteString(fmt.Sprintf("Completed at: %s\n", time.Now().Format(time.RFC3339)))
	logContent.WriteString(fmt.Sprintf("Source: %s\n", remotePath))
	logContent.WriteString(fmt.Sprintf("Destination: %s\n", localPath))
	logContent.WriteString(fmt.Sprintf("Total Size: %s\n", totalSizeStr))
	logContent.WriteString(fmt.Sprintf("Total Files: %d\n\n", len(fileInfos)))
	logContent.WriteString("File Details\n============\n\n")
	logContent.WriteString(strings.Join(fileInfos, "\n\n"))

	if err := os.WriteFile(logPath, []byte(logContent.String()), 0644); err != nil {
		log.Printf("Failed to write summary log: %v", err)
	}
}
