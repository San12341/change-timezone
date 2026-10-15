// change-timezone：根据代理出口 IP 自动切换系统时区（Windows / macOS）。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"change-timezone/internal/app"
	"change-timezone/internal/config"
	"change-timezone/internal/ipq"
	"change-timezone/internal/logx"
	"change-timezone/internal/proxyd"
	"change-timezone/internal/tz"
)

const usageText = `change-timezone —— 根据代理出口 IP 自动切换系统时区

用法：
  change-timezone apply  [--dry-run] [--force]     检测代理并切换时区
  change-timezone restore [备份文件名] [--dry-run]  恢复备份的时区（默认最新一份）
  change-timezone list                             列出全部备份
  change-timezone version                          显示版本

选项：
  --dry-run   仅演示，不修改系统设置、不写备份
  --force     跳过代理开关检测（适用于 TUN 模式）`

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, usageText)
		return app.ExitUsage
	}
	switch args[0] {
	case "apply":
		var force, dry bool
		for _, a := range args[1:] {
			switch a {
			case "--dry-run":
				dry = true
			case "--force":
				force = true
			default:
				fmt.Fprintf(os.Stderr, "未知参数：%s\n\n%s\n", a, usageText)
				return app.ExitUsage
			}
		}
		return newEnv().Apply(force, dry)
	case "restore":
		var name string
		var dry bool
		for _, a := range args[1:] {
			if a == "--dry-run" {
				dry = true
				continue
			}
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(os.Stderr, "未知参数：%s\n\n%s\n", a, usageText)
				return app.ExitUsage
			}
			if name != "" {
				fmt.Fprintln(os.Stderr, "只能指定一个备份文件名\n"+usageText)
				return app.ExitUsage
			}
			name = a
		}
		return newEnv().Restore(name, dry)
	case "list":
		if len(args) > 1 {
			fmt.Fprintln(os.Stderr, "list 不接受参数\n"+usageText)
			return app.ExitUsage
		}
		return newEnv().List()
	case "version":
		fmt.Printf("change-timezone v%s（%s/%s）\n", config.Version, runtime.GOOS, runtime.GOARCH)
		return app.ExitOK
	case "help", "-h", "--help":
		fmt.Println(usageText)
		return app.ExitOK
	default:
		fmt.Fprintf(os.Stderr, "未知子命令：%s\n\n%s\n", args[0], usageText)
		return app.ExitUsage
	}
}

func newEnv() app.Env {
	dataDir, err := config.DataDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：无法定位用户主目录：%v（退出码 1）\n", err)
		os.Exit(app.ExitUnknown)
	}
	logPath := filepath.Join(dataDir, config.LogFileName)
	lg, err := logx.Open(logPath, config.LogMaxBytes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "警告：无法写入日志文件 %s：%v（继续运行）\n", logPath, err)
		lg = logx.Nop()
	}
	return app.Env{
		Proxy:   proxyd.Detect,
		Query:   ipq.Query,
		TZ:      tz.RealOps{},
		DataDir: dataDir,
		Log:     lg,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	}
}
