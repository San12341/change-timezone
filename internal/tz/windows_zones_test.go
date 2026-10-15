package tz

import "testing"

func TestKnownMappings(t *testing.T) {
	cases := map[string]string{
		"Asia/Shanghai":       "China Standard Time",
		"Asia/Hong_Kong":      "China Standard Time",
		"America/New_York":    "Eastern Standard Time",
		"America/Los_Angeles": "Pacific Standard Time",
		"Europe/London":       "GMT Standard Time",
		"Europe/Berlin":       "W. Europe Standard Time",
		"Asia/Tokyo":          "Tokyo Standard Time",
		"Australia/Sydney":    "AUS Eastern Standard Time",
		"Asia/Dubai":          "Arabian Standard Time",
		"America/Sao_Paulo":   "E. South America Standard Time",
		"UTC":                 "UTC",
		"Asia/Calcutta":       "India Standard Time",
	}
	for iana, want := range cases {
		if got, ok := IanaToWindows(iana); !ok || got != want {
			t.Errorf("%s → %q, want %q", iana, got, want)
		}
	}
	if _, ok := IanaToWindows("Mars/Olympus_Mons"); ok {
		t.Error("未知时区不应映射成功")
	}
}

func TestReverseMappings(t *testing.T) {
	cases := map[string]string{
		"China Standard Time":       "Asia/Shanghai",
		"Eastern Standard Time":     "America/New_York",
		"GMT Standard Time":         "Europe/London",
		"Tokyo Standard Time":       "Asia/Tokyo",
		"AUS Eastern Standard Time": "Australia/Sydney",
	}
	for win, want := range cases {
		if got, ok := WindowsToIana(win); !ok || got != want {
			t.Errorf("%s → %q, want %q", win, got, want)
		}
	}
}

// 反向映射的每个规范 IANA 名必须能正向映射回同一 Windows ID
// （仅“Tasmania Standard Time”例外：旧版 ID，正向已并入 AUS Eastern）。
func TestReverseForwardConsistency(t *testing.T) {
	for win, iana := range windowsToIanaCanonical {
		if win == "Tasmania Standard Time" {
			continue
		}
		got, ok := ianaToWindows[iana]
		if !ok {
			t.Errorf("反向映射的 %q 不在正向表中", iana)
			continue
		}
		if got != win {
			t.Errorf("%q 正向映射到 %q，与反向 %q 不一致", iana, got, win)
		}
	}
}

// 正向表的每个 Windows ID 都应有反向映射（读取当前时区时需要）。
func TestForwardValuesHaveReverse(t *testing.T) {
	seen := make(map[string]bool)
	for _, win := range ianaToWindows {
		if seen[win] {
			continue
		}
		seen[win] = true
		if _, ok := windowsToIanaCanonical[win]; !ok {
			t.Errorf("正向表的 %q 缺少反向映射", win)
		}
	}
}
