package proxyd

import "testing"

const scutilOn = `<dictionary> {
  HTTPEnable : 1
  HTTPProxy : 127.0.0.1
  HTTPPort : 7897
  HTTPSEnable : 1
  HTTPSProxy : 127.0.0.1
  HTTPSPort : 7897
  SOCKSEnable : 1
  SOCKSProxy : 127.0.0.1
  SOCKSPort : 7897
  ProxyAutoConfigEnable : 0
}`

const scutilOff = `<dictionary> {
  HTTPEnable : 0
  HTTPProxy : 127.0.0.1
  HTTPPort : 7897
  HTTPSEnable : 0
  HTTPSProxy : 127.0.0.1
  HTTPSPort : 7897
  SOCKSEnable : 0
  ExceptionsList : <array> {
    0 : 127.0.0.1
  }
}`

func TestParseScutilDict(t *testing.T) {
	m := ParseScutilDict(scutilOn)
	if m["HTTPSEnable"] != "1" || m["HTTPSProxy"] != "127.0.0.1" || m["HTTPSPort"] != "7897" {
		t.Fatalf("解析结果不正确: %#v", m)
	}
	if _, ok := m["<dictionary> {"]; ok {
		t.Fatalf("不应包含装饰行: %#v", m)
	}
}

func TestInfoFromScutilDict(t *testing.T) {
	cases := []struct {
		name    string
		dict    map[string]string
		enabled bool
		addr    string
		kind    string
		pac     bool
	}{
		{"全部开启", map[string]string{
			"HTTPEnable": "1", "HTTPProxy": "127.0.0.1", "HTTPPort": "7897",
			"HTTPSEnable": "1", "HTTPSProxy": "127.0.0.1", "HTTPSPort": "7897",
		}, true, "http://127.0.0.1:7897", "http", false},
		{"仅 SOCKS", map[string]string{
			"SOCKSEnable": "1", "SOCKSProxy": "127.0.0.1", "SOCKSPort": "1080",
		}, true, "socks5://127.0.0.1:1080", "socks5", false},
		{"仅 HTTP", map[string]string{
			"HTTPEnable": "1", "HTTPProxy": "127.0.0.1", "HTTPPort": "8888",
			"HTTPSEnable": "0",
		}, true, "http://127.0.0.1:8888", "http", false},
		{"全关", map[string]string{"HTTPEnable": "0", "HTTPSEnable": "0", "SOCKSEnable": "0"}, false, "", "", false},
		{"仅 PAC", map[string]string{
			"HTTPEnable": "0", "HTTPSEnable": "0", "SOCKSEnable": "0",
			"ProxyAutoConfigEnable": "1",
		}, false, "", "", true},
		{"开启但缺端口", map[string]string{
			"HTTPEnable": "1", "HTTPProxy": "127.0.0.1",
		}, true, "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := InfoFromScutilDict(c.dict)
			if got.Enabled != c.enabled || got.Addr != c.addr || got.Kind != c.kind || got.PACOnly != c.pac {
				t.Fatalf("got=%+v want enabled=%v addr=%q kind=%q pac=%v", got, c.enabled, c.addr, c.kind, c.pac)
			}
		})
	}
}

func TestParseRegValueOutput(t *testing.T) {
	out := "\r\nHKEY_CURRENT_USER\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings\r\n    ProxyEnable    REG_DWORD    0x1\r\n\r\n"
	if v, ok := ParseRegValueOutput(out, "ProxyEnable"); !ok || v != "0x1" {
		t.Fatalf("got %q %v", v, ok)
	}
	out2 := "    ProxyServer    REG_SZ    127.0.0.1:7890\r\n"
	if v, ok := ParseRegValueOutput(out2, "ProxyServer"); !ok || v != "127.0.0.1:7890" {
		t.Fatalf("got %q %v", v, ok)
	}
	if _, ok := ParseRegValueOutput(out2, "NotExist"); ok {
		t.Fatal("不存在的值不应找到")
	}
}

func TestInfoFromRegistry(t *testing.T) {
	cases := []struct {
		name    string
		enable  bool
		server  string
		acURL   string
		enabled bool
		addr    string
		kind    string
		pac     bool
	}{
		{"全局代理", true, "127.0.0.1:7890", "", true, "http://127.0.0.1:7890", "http", false},
		{"分协议代理", true, "http=a:1;https=b:2;socks=c:3", "", true, "http://b:2", "http", false},
		{"仅 SOCKS 分协议", true, "ftp=x:9;socks=127.0.0.1:1080", "", true, "socks5://127.0.0.1:1080", "socks5", false},
		{"开启但服务器为空", true, "", "", true, "", "", false},
		{"关闭且无 PAC", false, "127.0.0.1:7890", "", false, "", "", false},
		{"关闭但配置 PAC", false, "", "http://pac/x.js", false, "", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := InfoFromRegistry(c.enable, c.server, c.acURL)
			if got.Enabled != c.enabled || got.Addr != c.addr || got.Kind != c.kind || got.PACOnly != c.pac {
				t.Fatalf("got=%+v want enabled=%v addr=%q kind=%q pac=%v", got, c.enabled, c.addr, c.kind, c.pac)
			}
		})
	}
}

func TestParseProxyServer(t *testing.T) {
	if addr, kind := ParseProxyServer("socks=127.0.0.1:1080"); addr != "socks5://127.0.0.1:1080" || kind != "socks5" {
		t.Fatalf("got %q %q", addr, kind)
	}
	if addr, kind := ParseProxyServer(""); addr != "" || kind != "" {
		t.Fatalf("空值应返回空: %q %q", addr, kind)
	}
}
