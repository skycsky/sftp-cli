package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config"
	"github.com/rclone/rclone/fs/config/configfile"
	"github.com/rclone/rclone/fs/sync"
	"github.com/urfave/cli/v2"
)

const (
	rcloneConfigFile = "./rclone.conf" // 配置文件路径
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
	configfile.Install() // 此处为无返回值函数，直接调用即可
	return nil
}

func rcloneSync(src, dst string, operation string) error {
	ctx := context.Background()
	fsSrc, err := fs.NewFs(ctx, src)
	if err != nil {
		return fmt.Errorf("failed to create source fs: %v", err)
	}

	fsDst, err := fs.NewFs(ctx, dst)
	if err != nil {
		return fmt.Errorf("failed to create destination fs: %v", err)
	}

	// 开始同步操作
	if operation == "upload" {
		err = sync.Sync(ctx, fsDst, fsSrc, false)
	} else if operation == "download" {
		err = sync.Sync(ctx, fsSrc, fsDst, false)
	}

	// 错误处理：如果出现错误，记录具体错误信息
	if err != nil {
		log.Printf("Sync operation failed for %s -> %s. Error: %v", src, dst, err)
		return fmt.Errorf("sync operation failed: %v", err)
	}

	log.Printf("Successfully completed %s from %s to %s", operation, src, dst)
	return nil
}

func rcloneList(remote string) error {
	ctx := context.Background()
	fsRemote, err := fs.NewFs(ctx, remote)
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
