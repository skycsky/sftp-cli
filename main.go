package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config"
	"github.com/rclone/rclone/fs/config/configfile"
	"github.com/rclone/rclone/fs/config/flags"
	"github.com/rclone/rclone/fs/sync"
	"github.com/urfave/cli/v2"
)

const (
	rcloneConfigFile = "./app.conf" // 配置文件路径
)

func main() {
	app := &cli.App{
		Name:  "sftp-cli",
		Usage: "A command-line tool for SFTP server operations",
		Commands: []*cli.Command{
			{
				Name:   "upload",
				Usage:  "Upload a file to the SFTP server",
				Action: uploadFile,
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "local", Aliases: []string{"l"}, Usage: "Local file path", Required: true},
					&cli.StringFlag{Name: "remote", Aliases: []string{"r"}, Usage: "Remote path on SFTP server", Required: true},
				},
			},
			{
				Name:   "download",
				Usage:  "Download a file from the SFTP server",
				Action: downloadFile,
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "remote", Aliases: []string{"r"}, Usage: "Remote file path on SFTP server", Required: true},
					&cli.StringFlag{Name: "local", Aliases: []string{"l"}, Usage: "Local destination path", Required: true},
				},
			},
			{
				Name:   "list",
				Usage:  "List files on the SFTP server",
				Action: listFiles,
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "remote", Aliases: []string{"r"}, Usage: "Remote directory path", Required: true},
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatalf("Error: %v\n", err)
	}
}

func uploadFile(c *cli.Context) error {
	localPath := c.String("local")
	remotePath := c.String("remote")

	// 使用 rclone API 上传文件
	return rcloneSync(localPath, fmt.Sprintf("sftp:%s", remotePath), "upload")
}

func downloadFile(c *cli.Context) error {
	remotePath := c.String("remote")
	localPath := c.String("local")

	// 使用 rclone API 下载文件
	return rcloneSync(fmt.Sprintf("sftp:%s", remotePath), localPath, "download")
}

func listFiles(c *cli.Context) error {
	remotePath := c.String("remote")

	// 使用 rclone API 列出文件
	return rcloneList(fmt.Sprintf("sftp:%s", remotePath))
}

// 初始化 rclone 配置
func initRclone() error {
	config.SetConfigPath(rcloneConfigFile)

	// 确保加载配置文件
	flags.ConfigFile = rcloneConfigFile
	if err := configfile.Install(); err != nil {
		return fmt.Errorf("failed to load config file: %v", err)
	}
	return nil
}

func rcloneSync(src, dst string, operation string) error {
	ctx := context.Background()
	fsSrc, err := fs.NewFs(src)
	if err != nil {
		return fmt.Errorf("failed to create source fs: %v", err)
	}

	fsDst, err := fs.NewFs(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination fs: %v", err)
	}

	var errorsList []string
	callback := func(src, dst string, size int64, err error) {
		if err != nil {
			errorsList = append(errorsList, fmt.Sprintf("File: %s, Error: %v", src, err))
		}
	}

	if operation == "upload" {
		err = sync.Sync(ctx, fsDst, fsSrc, false, callback)
	} else if operation == "download" {
		err = sync.Sync(ctx, fsSrc, fsDst, false, callback)
	}

	if err != nil {
		return fmt.Errorf("sync operation failed: %v", err)
	}

	if len(errorsList) > 0 {
		for _, errorMsg := range errorsList {
			log.Printf("Upload Error: %s", errorMsg)
		}
		return fmt.Errorf("some files failed during %s", operation)
	}

	log.Printf("Successfully completed %s from %s to %s", operation, src, dst)
	return nil
}

func rcloneList(remote string) error {
	ctx := context.Background()
	fsRemote, err := fs.NewFs(remote)
	if err != nil {
		return fmt.Errorf("failed to create remote fs: %v", err)
	}

	dirEntries, err := fsRemote.List(ctx, "")
	if err != nil {
		return fmt.Errorf("failed to list remote directory: %v", err)
	}

	log.Printf("Files in %s:", remote)
	for _, entry := range dirEntries {
		log.Printf("  %s", entry.Remote())
	}

	return nil
}
