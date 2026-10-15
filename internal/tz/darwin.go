//go:build darwin

package tz

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func current() (string, error) {
	if p, err := os.Readlink("/etc/localtime"); err == nil {
		if i := strings.LastIndex(p, "zoneinfo/"); i >= 0 {
			return p[i+len("zoneinfo/"):], nil
		}
	}
	out, err := exec.Command("systemsetup", "-gettimezone").Output()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if v, ok := strings.CutPrefix(strings.TrimSpace(line), "Time Zone:"); ok {
				if name := strings.TrimSpace(v); name != "" {
					return name, nil
				}
			}
		}
	}
	return "", fmt.Errorf("无法读取当前系统时区")
}

// validate 借助系统 zoneinfo 数据校验 IANA 时区名（无需 root，dry-run 也可用）。
func validate(iana string) error {
	if _, err := time.LoadLocation(iana); err != nil {
		return fmt.Errorf("时区 %q 不是有效的 IANA 时区", iana)
	}
	return nil
}

func set(iana string) error {
	out, err := exec.Command("systemsetup", "-settimezone", iana).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemsetup -settimezone 执行失败: %v（%s）", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func verify(iana string) (string, bool, error) {
	cur, err := current()
	if err != nil {
		return "", false, err
	}
	return cur, cur == iana, nil
}

// needElevation macOS 修改时区需要 sudo（需求文档决策 5）。
func needElevation() bool { return os.Geteuid() != 0 }

func sameZone(a, b string) bool { return a == b }
