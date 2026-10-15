//go:build darwin

package app

func (e Env) printAutoTZHint() {
	e.out(`提示：若系统开启了“自动时区”（定位服务），系统可能稍后改回，请在 系统设置 → 通用 → 日期与时间 中关闭。`)
}
