//go:build windows

package main

import "syscall"

// 让控制台以 UTF-8 输出中文。
func init() {
	k32 := syscall.NewLazyDLL("kernel32.dll")
	setOut := k32.NewProc("SetConsoleOutputCP")
	setIn := k32.NewProc("SetConsoleCP")
	_, _, _ = setOut.Call(65001)
	_, _, _ = setIn.Call(65001)
}
