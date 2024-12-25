package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	// rcloneConfigFile = "./app.conf"        // 配置文件路径
	watchDir   = "./watch"           // 监控目录
	sftpRemote = "sftp:/remote/path" // 远程 SFTP 路径
)

func daemon() {
	// 初始化 rclone 配置
	if err := initRclone(); err != nil {
		log.Fatalf("Failed to initialize rclone: %v", err)
	}

	// 检查监控目录是否存在
	if _, err := os.Stat(watchDir); os.IsNotExist(err) {
		log.Fatalf("Watch directory does not exist: %s", watchDir)
	}

	// 启动文件监控
	log.Printf("Starting to watch directory: %s", watchDir)
	if err := startWatching(watchDir); err != nil {
		log.Fatalf("Failed to watch directory: %v", err)
	}
}

// func initRclone() error {
// 	config.SetConfigPath(rcloneConfigFile)

// 	// 确保加载配置文件
// 	configfile.Install() // 无返回值，直接调用
// 	return nil
// }

func startWatching(directory string) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create watcher: %v", err)
	}
	defer watcher.Close()

	filesBeingWritten := make(map[string]struct{})

	// 添加目录到监控列表
	if err := watcher.Add(directory); err != nil {
		return fmt.Errorf("failed to watch directory: %v", err)
	}

	// 处理文件事件
	for {
		select {
		case event := <-watcher.Events:
			if event.Op&(fsnotify.Create|fsnotify.Write) != 0 {
				filePath := event.Name
				// 跳过已经处理的写入文件
				if _, exists := filesBeingWritten[filePath]; exists {
					continue
				}
				filesBeingWritten[filePath] = struct{}{}

				go func(filePath string) {
					defer delete(filesBeingWritten, filePath)
					// 延迟一段时间，等待文件写入完成
					time.Sleep(2 * time.Second)

					// 确认文件大小稳定
					if isFileStable(filePath) {
						log.Printf("Detected new file ready for upload: %s", filePath)
						if err := uploadNewFile(filePath); err != nil {
							log.Printf("Failed to upload file: %s, error: %v", filePath, err)
						}
					} else {
						log.Printf("File %s is not stable, skipping upload", filePath)
					}
				}(filePath)
			}
		case err := <-watcher.Errors:
			log.Printf("Watcher error: %v", err)
		}
	}
}

// 判断文件是否稳定（文件大小是否在一段时间内未变化）
func isFileStable(filePath string) bool {
	initialInfo, err := os.Stat(filePath)
	if err != nil {
		log.Printf("Failed to stat file %s: %v", filePath, err)
		return false
	}

	time.Sleep(1 * time.Second)

	finalInfo, err := os.Stat(filePath)
	if err != nil {
		log.Printf("Failed to stat file %s: %v", filePath, err)
		return false
	}

	// 比较文件大小是否一致
	return initialInfo.Size() == finalInfo.Size()
}

func uploadNewFile(filePath string) error {
	// 检查文件是否是普通文件
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("failed to stat file: %v", err)
	}
	if info.IsDir() {
		return nil // 忽略目录
	}

	// 上传文件到远程路径
	remotePath := fmt.Sprintf("%s/%s", sftpRemote, info.Name())
	log.Printf("Uploading %s to %s", filePath, remotePath)
	return rcloneSync(filePath, remotePath, "upload")
}
