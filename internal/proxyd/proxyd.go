// Package proxyd 检测系统代理状态（Windows 注册表 / macOS scutil）。
package proxyd

import (
	"net"
	"strings"
)

// Info 描述系统代理状态。
type Info struct {
	Enabled bool   // 是否开启了固定系统代理
	Addr    string // 代理地址，形如 http://127.0.0.1:7890 或 socks5://127.0.0.1:7890；空表示无
	Kind    string // "http"、"socks5" 或 ""
	PACOnly bool   // 仅开启了 PAC 自动配置（无固定代理地址，本程序不支持）
}

// Detector 是代理检测接口（便于测试注入）。
type Detector interface {
	Detect() (Info, error)
}

// ParseScutilDict 解析 `scutil --proxy` 的输出为键值表。
func ParseScutilDict(out string) map[string]string {
	m := make(map[string]string)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "<dictionary> {" || line == "}" {
			continue
		}
		if k, v, ok := strings.Cut(line, ":"); ok {
			m[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return m
}

// InfoFromScutilDict 根据解析后的 scutil 键值表得到代理状态。
func InfoFromScutilDict(m map[string]string) Info {
	info := Info{
		Enabled: m["HTTPSEnable"] == "1" || m["HTTPEnable"] == "1" || m["SOCKSEnable"] == "1",
	}
	switch {
	case withPort(m, "HTTPS"):
		info.Addr, info.Kind = "http://"+net.JoinHostPort(m["HTTPSProxy"], m["HTTPSPort"]), "http"
	case withPort(m, "HTTP"):
		info.Addr, info.Kind = "http://"+net.JoinHostPort(m["HTTPProxy"], m["HTTPPort"]), "http"
	case withPort(m, "SOCKS"):
		info.Addr, info.Kind = "socks5://"+net.JoinHostPort(m["SOCKSProxy"], m["SOCKSPort"]), "socks5"
	}
	if !info.Enabled && m["ProxyAutoConfigEnable"] == "1" {
		info.PACOnly = true
	}
	return info
}

func withPort(m map[string]string, prefix string) bool {
	return m[prefix+"Enable"] == "1" && m[prefix+"Proxy"] != "" && m[prefix+"Port"] != ""
}

// ParseRegValueOutput 在 `reg query` 输出中查找指定值。
// 行形如 "    ProxyEnable    REG_DWORD    0x1"。
func ParseRegValueOutput(out, name string) (string, bool) {
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		for i := 0; i+2 < len(f); i++ {
			if strings.EqualFold(f[i], name) {
				return f[i+2], true
			}
		}
	}
	return "", false
}

// InfoFromRegistry 根据注册表值得到代理状态。
func InfoFromRegistry(enable bool, server, autoConfigURL string) Info {
	info := Info{Enabled: enable}
	if enable && server != "" {
		info.Addr, info.Kind = ParseProxyServer(server)
	}
	if !info.Enabled && autoConfigURL != "" {
		info.PACOnly = true
	}
	return info
}

// ParseProxyServer 解析注册表 ProxyServer 的取值：
// 全局形如 "127.0.0.1:7890"，分协议形如 "http=a:1;https=b:2;socks=c:3"。
func ParseProxyServer(s string) (addr, kind string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	if strings.Contains(s, "=") {
		m := make(map[string]string)
		for _, part := range strings.Split(s, ";") {
			if k, v, ok := strings.Cut(part, "="); ok {
				m[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
			}
		}
		for _, k := range []string{"https", "http"} {
			if v := m[k]; v != "" {
				return "http://" + v, "http"
			}
		}
		if v := m["socks"]; v != "" {
			return "socks5://" + v, "socks5"
		}
		return "", ""
	}
	return "http://" + s, "http"
}
