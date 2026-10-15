//go:build windows

package proxyd

import (
	"os/exec"
	"strconv"
	"strings"
)

const regKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`

// Detect 读取 Windows 注册表中的系统代理设置。
func Detect() (Info, error) {
	enable := false
	if v, ok := regValue("ProxyEnable"); ok {
		if n, err := strconv.ParseUint(v, 0, 32); err == nil {
			enable = n == 1
		}
	}
	server, _ := regValue("ProxyServer")
	autoConfig, _ := regValue("AutoConfigURL")
	return InfoFromRegistry(enable, server, autoConfig), nil
}

func regValue(name string) (string, bool) {
	out, err := exec.Command("reg", "query", regKey, "/v", name).Output()
	if err != nil {
		return "", false // 值不存在视为未配置
	}
	v, ok := ParseRegValueOutput(string(out), name)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(v), true
}
