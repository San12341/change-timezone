//go:build windows

package tz

import (
	"fmt"
	"os/exec"
	"strings"
)

func current() (string, error) {
	out, err := exec.Command("tzutil", "/g").Output()
	if err != nil {
		return "", fmt.Errorf("tzutil /g 执行失败: %w", err)
	}
	win := strings.TrimSpace(string(out))
	if iana, ok := WindowsToIana(win); ok {
		return iana, nil
	}
	// 极少数系统时区不在映射表内：退回 Windows ID，仅用于记录与显示。
	return win, nil
}

func validate(iana string) error {
	if _, ok := IanaToWindows(iana); !ok {
		return fmt.Errorf("IANA 时区 %q 无对应的 Windows 时区", iana)
	}
	return nil
}

func set(iana string) error {
	win, ok := IanaToWindows(iana)
	if !ok {
		return fmt.Errorf("IANA 时区 %q 无对应的 Windows 时区", iana)
	}
	out, err := exec.Command("tzutil", "/s", win).CombinedOutput()
	if err != nil {
		return fmt.Errorf("tzutil /s 执行失败: %v（%s）", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func verify(iana string) (string, bool, error) {
	out, err := exec.Command("tzutil", "/g").Output()
	if err != nil {
		return "", false, fmt.Errorf("tzutil /g 执行失败: %w", err)
	}
	gotWin := strings.TrimSpace(string(out))
	want, _ := IanaToWindows(iana)
	gotIana, _ := WindowsToIana(gotWin)
	return gotIana, gotWin == want, nil
}

// needElevation Windows 由 UAC manifest 保证管理员权限，无需运行时检查。
func needElevation() bool { return false }

func sameZone(a, b string) bool {
	wa, oka := IanaToWindows(a)
	wb, okb := IanaToWindows(b)
	if oka && okb {
		return wa == wb
	}
	return a == b
}
