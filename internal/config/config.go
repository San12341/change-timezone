// Package config 集中管理程序内置常量（需求文档约定：不提供配置项覆盖）。
package config

import (
	"os"
	"path/filepath"
	"time"
)

// Version 为 go build 直接编译时的默认值；make build 会用 git tag 通过
// -ldflags "-X change-timezone/internal/config.Version=..." 注入实际版本。
var Version = "1.0.0"

const (
	DataDirName    = ".change-timezone"
	BackupsDirName = "backups"
	StateFileName  = "state.json"
	LogFileName    = "change-timezone.log"
	LogMaxBytes    = 1 << 20 // 日志轮转阈值 1 MB
	KeepBackups    = 20      // 备份保留份数
	IPQueryURL     = "https://ipwho.is/"
	QueryTimeout   = 8 * time.Second
)

// DataDir 返回数据目录 ~/.change-timezone。
func DataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, DataDirName), nil
}
