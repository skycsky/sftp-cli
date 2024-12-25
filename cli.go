package main

import (
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
)

func sftpcli() {
	// 定义 rclone 命令和参数
	configFile := "/path/to/config"   // 替换为你的 rclone 配置文件路径
	remoteName := "remote:/temp"      // 替换为你的远程名称，例如 remote:/temp 表示上传到 temp 目录
	localPath := "/path/to/local"     // 替换为本地路径
	logFile := "/path/to/logfile.log" // 替换为日志文件路径

	// 打开日志文件
	logFileHandle, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer logFileHandle.Close()

	// 创建多路输出，将日志同时输出到文件和控制台
	multiWriter := io.MultiWriter(os.Stdout, logFileHandle)
	log.SetOutput(multiWriter)

	// 捕获系统信号以实现优雅关闭
	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalChannel
		log.Println("Daemon shutting down gracefully...")
		os.Exit(0)
	}()

	// 创建文件监控器
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatalf("Failed to create file watcher: %v", err)
	}
	defer watcher.Close()

	// 添加需要监控的目录
	err = watcher.Add(localPath)
	if err != nil {
		log.Fatalf("Failed to add directory to watcher: %v", err)
	}

	log.Printf("Watching directory: %s\n", localPath)

	// 定时任务触发器
	ticker := time.NewTicker(10 * time.Minute) // 每10分钟触发一次
	defer ticker.Stop()

	// 主事件循环
	for {
		select {
		case event := <-watcher.Events:
			if event.Op&(fsnotify.Create|fsnotify.Write) != 0 {
				log.Printf("Detected new or modified file: %s\n", event.Name)

				// 确保文件完成拷贝
				if !isFileReady(event.Name) {
					log.Printf("File %s is not ready yet, skipping...\n", event.Name)
					continue
				}

				// 执行同步
				syncDirectory(configFile, localPath, remoteName, multiWriter)
			}
		case <-ticker.C:
			log.Println("Scheduled sync triggered.")
			// 执行同步
			syncDirectory(configFile, localPath, remoteName, multiWriter)
		case err := <-watcher.Errors:
			log.Printf("Watcher error: %v\n", err)
		case <-signalChannel:
			log.Println("Daemon shutting down gracefully...")
			return
		}
	}
}

// 检查文件是否准备好（通过检测文件是否稳定一段时间）
func isFileReady(filePath string) bool {
	info, err := os.Stat(filePath)
	if err != nil {
		return false
	}
	currentSize := info.Size()
	time.Sleep(2 * time.Second)
	info, err = os.Stat(filePath)
	if err != nil {
		return false
	}
	return info.Size() == currentSize
}

// 执行目录同步
func syncDirectory(configFile, localPath, remoteName string, multiWriter io.Writer) {
	cmd := exec.Command("rclone", "sync", localPath, remoteName, "--config", configFile, "--progress")
	cmd.Stdout = multiWriter
	cmd.Stderr = multiWriter
	log.Printf("Executing command: %s\n", cmd.String())
	err := cmd.Run()
	if err != nil {
		log.Printf("Error running rclone command: %v\n", err)
	} else {
		log.Println("Directory synced successfully")
	}
}
