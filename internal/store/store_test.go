package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteBackupNameAndContent(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 15, 16, 55, 30, 0, time.UTC)
	path, err := WriteBackup(dir, "Asia/Shanghai", now)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "20261015-165530-Asia_Shanghai" {
		t.Fatalf("文件名不正确: %s", path)
	}
	got, err := ReadBackup(path)
	if err != nil || got != "Asia/Shanghai" {
		t.Fatalf("内容不正确: %q %v", got, err)
	}
}

func TestWriteBackupCollisionSuffix(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	p1, err := WriteBackup(dir, "Asia/Shanghai", now)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := WriteBackup(dir, "Asia/Shanghai", now)
	if err != nil {
		t.Fatal(err)
	}
	if p1 == p2 {
		t.Fatalf("同秒重名应生成不同文件: %s", p1)
	}
	if filepath.Base(p2) != filepath.Base(p1)+"-2" {
		t.Fatalf("后缀不正确: %s vs %s", p2, p1)
	}
}

func TestListBackupsSorted(t *testing.T) {
	dir := t.TempDir()
	for _, tz := range []string{"Asia/Shanghai", "Europe/London", "America/New_York"} {
		if _, err := WriteBackup(dir, tz, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	names, err := ListBackups(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 3 {
		t.Fatalf("应有 3 份备份: %v", names)
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Fatalf("应升序排序: %v", names)
		}
	}
}

func TestCleanupKeepsNewest(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 25; i++ {
		// 手工造 25 份按时间递增的备份
		name := "20260101-0000" + string(rune('0'+i)) + "-00-TZ"
		if i >= 10 {
			name = "20260101-0000" + string(rune('a'+i-10)) + "-00-TZ"
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte("X/Y\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	removed, err := Cleanup(dir, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 5 {
		t.Fatalf("应删除 5 份: %v", removed)
	}
	names, _ := ListBackups(dir)
	if len(names) != 20 {
		t.Fatalf("应保留 20 份: %d", len(names))
	}
}

func TestStateRoundtrip(t *testing.T) {
	dir := t.TempDir()
	if s, err := LoadState(dir); err != nil || s.LastApplied != "" {
		t.Fatalf("初始状态应为零值: %+v %v", s, err)
	}
	want := State{LastApplied: "America/New_York", LastAppliedAt: "2026-10-15T16:55:31+08:00"}
	if err := SaveState(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadState(dir)
	if err != nil || got != want {
		t.Fatalf("往返不一致: %+v %v", got, err)
	}
}

func TestReadBackupEmpty(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "b")
	os.WriteFile(p, []byte("  \n"), 0o644)
	if _, err := ReadBackup(p); err == nil {
		t.Fatal("空内容应报错")
	}
}
