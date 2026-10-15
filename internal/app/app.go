// Package app 实现 apply / restore / list 的业务流程与退出码规范（需求文档第 4–7 节）。
package app

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"change-timezone/internal/config"
	"change-timezone/internal/ipq"
	"change-timezone/internal/logx"
	"change-timezone/internal/proxyd"
	"change-timezone/internal/store"
)

// 退出码规范（需求文档第 7 节）。
const (
	ExitOK           = 0  // 成功（含幂等跳过、dry-run）
	ExitUnknown      = 1  // 未知错误
	ExitNoProxy      = 2  // 未检测到系统代理
	ExitQueryFailed  = 3  // IP 查询失败
	ExitNoPermission = 4  // 权限不足
	ExitBadTimezone  = 5  // 时区数据异常
	ExitBackupFailed = 6  // 备份/文件操作失败
	ExitSetFailed    = 7  // 时区修改失败
	ExitPACOnly      = 8  // PAC 模式限制
	ExitUsage        = 64 // 用法错误
)

// QueryFunc 是 IP 归属地查询函数签名（ipq.Query 即满足）。
type QueryFunc func(rawURL, proxyAddr string, timeout time.Duration) (ipq.Result, error)

// DetectFunc 是系统代理检测函数签名（proxyd.Detect 即满足）。
type DetectFunc func() (proxyd.Info, error)

type TZOps interface {
	Current() (string, error)
	Validate(iana string) error
	Set(iana string) error
	Verify(iana string) (actual string, ok bool, err error)
	NeedElevation() bool
	SameZone(a, b string) bool
}

// Env 聚合一次运行所需的全部依赖（便于测试注入）。
type Env struct {
	Proxy   DetectFunc
	Query   QueryFunc
	TZ      TZOps
	DataDir string
	Log     *logx.Logger
	Stdout  io.Writer
	Stderr  io.Writer
}

func (e Env) out(format string, a ...any)  { fmt.Fprintf(e.Stdout, format+"\n", a...) }
func (e Env) errf(format string, a ...any) { fmt.Fprintf(e.Stderr, format+"\n", a...) }

func (e Env) fail(code int, reason, advice string) int {
	e.errf("错误：%s（退出码 %d）", reason, code)
	if advice != "" {
		e.errf("建议：%s", advice)
	}
	e.Log.Error("failed", "reason", reason, "exit_code", fmt.Sprint(code))
	return code
}

func (e Env) backupDir() string { return filepath.Join(e.DataDir, config.BackupsDirName) }

// Apply 执行主流程。force 跳过代理开关检测（TUN 模式）；dry 仅演示，不产生任何系统改动。
func (e Env) Apply(force, dry bool) int {
	if dry {
		e.out("[DRY-RUN] 仅演示，不会修改系统设置、不写入备份")
	}

	// 步骤 1：检测系统代理
	var info proxyd.Info
	if !force {
		var err error
		info, err = e.Proxy()
		if err != nil {
			return e.fail(ExitUnknown, "检测系统代理失败: "+err.Error(), "")
		}
		e.Log.Info("proxy_detected", "enabled", fmt.Sprint(info.Enabled), "addr", info.Addr, "pac_only", fmt.Sprint(info.PACOnly))
		if info.PACOnly {
			return e.fail(ExitPACOnly, "检测到 PAC 自动配置代理，且未配置固定代理服务器地址",
				"本程序暂不支持 PAC；可改用全局系统代理，或使用 TUN 模式并加 --force 运行")
		}
		if !info.Enabled {
			return e.fail(ExitNoProxy, "未检测到系统代理", "若使用 TUN 模式代理，可加 --force 重试")
		}
		e.out("[1/5] 检测系统代理……已开启（%s）", describeProxy(info))
	} else {
		var err error
		if info, err = e.Proxy(); err != nil {
			e.Log.Error("proxy_detect_failed_in_force_mode", "err", err.Error())
		} else {
			e.Log.Info("proxy_detected_force", "addr", info.Addr)
		}
		if info.Addr != "" {
			e.out("[1/5] 检测系统代理……跳过（--force），将使用代理 %s", describeProxy(info))
		} else {
			e.out("[1/5] 检测系统代理……跳过（--force），未发现固定代理地址，将直连查询（适用于 TUN 模式）")
		}
	}

	// 步骤 2：备份当前时区（失败路径回滚，保证“失败时零残留”）
	cur, err := e.TZ.Current()
	if err != nil {
		return e.fail(ExitUnknown, "读取当前系统时区失败: "+err.Error(), "")
	}
	e.Log.Info("tz_current", "tz", cur)

	state, err := store.LoadState(e.DataDir)
	if err != nil {
		e.Log.Error("state_load_failed", "err", err.Error())
	}
	needBackup := state.LastApplied == "" || cur != state.LastApplied

	var backupPath string
	switch {
	case !needBackup:
		e.out("[2/5] 备份当前时区……跳过（当前时区由本程序设置，无需备份）")
	case dry:
		e.out("[2/5] 备份当前时区……（仅演示）将备份 %s → %s", cur,
			filepath.Join(e.backupDir(), store.BackupName(cur, time.Now())))
	default:
		if backupPath, err = store.WriteBackup(e.backupDir(), cur, time.Now()); err != nil {
			return e.fail(ExitBackupFailed, "写入备份失败: "+err.Error(), "")
		}
		e.Log.Info("backup_written", "path", backupPath, "tz", cur)
		if removed, err := store.Cleanup(e.backupDir(), config.KeepBackups); err != nil {
			e.Log.Error("backup_cleanup_failed", "err", err.Error())
		} else if len(removed) > 0 {
			e.Log.Info("backup_cleaned", "removed", strings.Join(removed, ","))
		}
		e.out("[2/5] 备份当前时区……已备份 %s → %s", cur, backupPath)
	}
	rollback := func() {
		if backupPath == "" {
			return
		}
		if err := store.RemoveBackup(backupPath); err != nil {
			e.Log.Error("backup_rollback_failed", "path", backupPath, "err", err.Error())
		} else {
			e.Log.Info("backup_rolled_back", "path", backupPath)
		}
	}

	// 步骤 3：查询代理出口 IP 归属地（请求显式走代理）
	res, err := e.Query(config.IPQueryURL, info.Addr, config.QueryTimeout)
	if err != nil {
		rollback()
		var qe *ipq.QueryError
		var de *ipq.DataError
		switch {
		case errors.As(err, &qe):
			return e.fail(ExitQueryFailed, "查询出口 IP 失败: "+err.Error(), "请检查网络与代理是否可用后重试")
		case errors.As(err, &de):
			return e.fail(ExitBadTimezone, "IP 归属时区数据异常: "+err.Error(), "已保持当前时区不变")
		default:
			return e.fail(ExitUnknown, "查询出口 IP 失败: "+err.Error(), "")
		}
	}
	e.Log.Info("ip_resolved", "ip", res.IP, "country", res.Country, "tz", res.TimezoneID)
	e.out("[3/5] 查询代理出口 IP……%s（%s）", res.IP, res.Country)

	// 步骤 4：校验目标时区
	target := res.TimezoneID
	if err := e.TZ.Validate(target); err != nil {
		rollback()
		return e.fail(ExitBadTimezone, "目标时区校验失败: "+err.Error(), "已保持当前时区不变")
	}
	e.out("[4/5] 匹配目标时区……%s", target)
	if e.TZ.SameZone(cur, target) {
		rollback() // 幂等场景不保留本次写入的备份（需求文档：幂等 = 不备份、不改状态）
		e.out("[5/5] 修改系统时区……已是目标时区，无需修改")
		e.Log.Info("tz_unchanged", "tz", target)
		return ExitOK
	}

	// 步骤 5：修改系统时区
	if dry {
		e.out("[5/5] 修改系统时区……跳过（仅演示）：将把 %s 改为 %s", cur, target)
		return ExitOK
	}
	if e.TZ.NeedElevation() {
		rollback()
		return e.fail(ExitNoPermission, "修改系统时区需要管理员权限", "请使用 sudo 运行本程序后重试")
	}
	if err := e.TZ.Set(target); err != nil {
		rollback()
		return e.fail(ExitSetFailed, "时区修改失败: "+err.Error(), "系统时区未变更")
	}
	actual, ok, err := e.TZ.Verify(target)
	if err != nil || !ok {
		if actual == "" {
			actual = "未知"
		}
		return e.fail(ExitSetFailed, fmt.Sprintf("时区修改后校验未通过（实际为 %s）", actual),
			"可使用 restore 子命令恢复备份")
	}
	if err := store.SaveState(e.DataDir, store.State{LastApplied: target, LastAppliedAt: time.Now().Format(time.RFC3339)}); err != nil {
		e.Log.Error("state_save_failed", "err", err.Error())
		e.errf("警告：状态文件写入失败（不影响本次时区修改）：%v", err)
	}
	e.Log.Info("tz_changed", "from", cur, "to", target)
	e.out("[5/5] 修改系统时区……成功（%s → %s）", cur, target)
	e.printAutoTZHint()
	return ExitOK
}

// Restore 从备份恢复时区。name 为空表示恢复最新一份。
func (e Env) Restore(name string, dry bool) int {
	if dry {
		e.out("[DRY-RUN] 仅演示，不会修改系统设置")
	}
	names, err := store.ListBackups(e.backupDir())
	if err != nil || len(names) == 0 {
		return e.fail(ExitBackupFailed, "备份目录中暂无可恢复的备份", "")
	}
	target := names[len(names)-1]
	if name != "" {
		if !contains(names, name) {
			return e.fail(ExitBackupFailed, "指定的备份不存在: "+name, "可使用 list 子命令查看全部备份")
		}
		target = name
	}
	tzName, err := store.ReadBackup(filepath.Join(e.backupDir(), target))
	if err != nil {
		return e.fail(ExitBackupFailed, "读取备份失败: "+err.Error(), "")
	}
	e.out("[1/3] 读取备份……目标：%s（%s）", target, tzName)

	cur, err := e.TZ.Current()
	if err != nil {
		return e.fail(ExitUnknown, "读取当前系统时区失败: "+err.Error(), "")
	}
	e.out("[2/3] 当前时区 %s → 目标 %s", cur, tzName)

	if e.TZ.SameZone(cur, tzName) {
		if !dry {
			if err := store.SaveState(e.DataDir, store.State{LastApplied: tzName, LastAppliedAt: time.Now().Format(time.RFC3339)}); err != nil {
				e.Log.Error("state_save_failed", "err", err.Error())
			}
		}
		e.out("[3/3] 恢复系统时区……已是该时区，无需修改")
		e.Log.Info("restore_noop", "tz", tzName)
		return ExitOK
	}
	if dry {
		e.out("[3/3] 恢复系统时区……跳过（仅演示）：将把 %s 恢复为 %s", cur, tzName)
		return ExitOK
	}
	if e.TZ.NeedElevation() {
		return e.fail(ExitNoPermission, "修改系统时区需要管理员权限", "请使用 sudo 运行本程序后重试")
	}
	if err := e.TZ.Set(tzName); err != nil {
		return e.fail(ExitSetFailed, "时区恢复失败: "+err.Error(), "")
	}
	if _, ok, err := e.TZ.Verify(tzName); err != nil || !ok {
		return e.fail(ExitSetFailed, "时区恢复后校验未通过", "")
	}
	if err := store.SaveState(e.DataDir, store.State{LastApplied: tzName, LastAppliedAt: time.Now().Format(time.RFC3339)}); err != nil {
		e.Log.Error("state_save_failed", "err", err.Error())
	}
	e.Log.Info("tz_restored", "from", cur, "to", tzName, "backup", target)
	e.out("[3/3] 恢复系统时区……成功（%s → %s）", cur, tzName)
	e.printAutoTZHint()
	return ExitOK
}

// List 按时间倒序列出全部备份。
func (e Env) List() int {
	names, err := store.ListBackups(e.backupDir())
	if err != nil || len(names) == 0 {
		e.out("暂无备份")
		return ExitOK
	}
	e.out("共 %d 份备份（新 → 旧）：", len(names))
	for i := len(names) - 1; i >= 0; i-- {
		tzName, err := store.ReadBackup(filepath.Join(e.backupDir(), names[i]))
		if err != nil {
			tzName = "（无法读取）"
		}
		e.out("%s    %s", names[i], tzName)
	}
	return ExitOK
}

func describeProxy(info proxyd.Info) string {
	if info.Addr == "" {
		return "已开启但未配置代理服务器地址，将直连查询"
	}
	if info.Kind == "socks5" {
		return "SOCKS5 " + strings.TrimPrefix(info.Addr, "socks5://")
	}
	return "HTTP " + strings.TrimPrefix(info.Addr, "http://")
}

func contains(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}
