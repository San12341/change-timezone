// Package store 管理备份文件与状态文件（防备份污染的状态机见需求文档 6.2）。
package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"change-timezone/internal/config"
)

// State 记录本程序最近一次成功设置的时区。
// 备份写入条件是“当前时区 != LastApplied”，保证备份目录只记录非本程序设置的时区。
type State struct {
	LastApplied   string `json:"last_applied"`
	LastAppliedAt string `json:"last_applied_at"`
}

// SanitizeName 把时区名转换为文件名安全的形式（/ → _）。
func SanitizeName(tz string) string { return strings.ReplaceAll(tz, "/", "_") }

// BackupName 生成备份文件名：YYYYMMDD-HHMMSS-<时区>。
func BackupName(tz string, now time.Time) string {
	return now.Format("20060102-150405") + "-" + SanitizeName(tz)
}

// WriteBackup 写入一份新备份（内容为时区名），返回文件路径。
// 同一秒内重名时追加 -2、-3……后缀。
func WriteBackup(backupDir, tz string, now time.Time) (string, error) {
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", err
	}
	base := BackupName(tz, now)
	path := filepath.Join(backupDir, base)
	for i := 2; ; i++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if os.IsExist(err) {
			path = filepath.Join(backupDir, base+"-"+strconv.Itoa(i))
			continue
		}
		if err != nil {
			return "", err
		}
		if _, err := f.WriteString(tz + "\n"); err != nil {
			f.Close()
			return "", err
		}
		return path, f.Close()
	}
}

// ListBackups 返回备份文件名列表（旧 → 新，按文件名排序）。
func ListBackups(backupDir string) ([]string, error) {
	ents, err := os.ReadDir(backupDir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(ents))
	for _, e := range ents {
		if !e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// ReadBackup 读取备份文件中的时区名（以文件内容为准，文件名仅作标签）。
func ReadBackup(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(b))
	if line == "" {
		return "", errors.New("备份文件内容为空")
	}
	return line, nil
}

// Cleanup 只保留最近 keep 份备份，返回被删除的文件名。
func Cleanup(backupDir string, keep int) ([]string, error) {
	names, err := ListBackups(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var removed []string
	for i := 0; i < len(names)-keep; i++ {
		if err := os.Remove(filepath.Join(backupDir, names[i])); err != nil {
			return removed, err
		}
		removed = append(removed, names[i])
	}
	return removed, nil
}

// RemoveBackup 删除一份备份（用于失败回滚）。
func RemoveBackup(path string) error { return os.Remove(path) }

// LoadState 读取状态文件；不存在时返回零值。
func LoadState(dataDir string) (State, error) {
	b, err := os.ReadFile(filepath.Join(dataDir, config.StateFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return State{}, nil
		}
		return State{}, err
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return State{}, err
	}
	return s, nil
}

// SaveState 原子写入状态文件。
func SaveState(dataDir string, s State) error {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dataDir, config.StateFileName+".tmp")
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dataDir, config.StateFileName))
}
