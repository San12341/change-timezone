//go:build darwin

package proxyd

import "os/exec"

// Detect 读取 macOS 系统代理设置（scutil --proxy）。
func Detect() (Info, error) {
	out, err := exec.Command("scutil", "--proxy").Output()
	if err != nil {
		return Info{}, err
	}
	return InfoFromScutilDict(ParseScutilDict(string(out))), nil
}
