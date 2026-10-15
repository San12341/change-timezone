// Package logx 提供追加写入的文件日志（超阈值轮转为 .old）。
package logx

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Logger struct {
	f *os.File
}

// Open 打开日志文件；超过 maxBytes 时先轮转。
func Open(path string, maxBytes int64) (*Logger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if maxBytes > 0 {
		if st, err := os.Stat(path); err == nil && st.Size() >= maxBytes {
			_ = os.Remove(path + ".old")
			_ = os.Rename(path, path+".old")
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return &Logger{f: f}, nil
}

// Nop 返回不落盘的空日志。
func Nop() *Logger { return &Logger{} }

func (l *Logger) Close() {
	if l != nil && l.f != nil {
		_ = l.f.Close()
	}
}

func (l *Logger) Info(event string, kv ...string)  { l.write("INFO", event, kv...) }
func (l *Logger) Error(event string, kv ...string) { l.write("ERROR", event, kv...) }

func (l *Logger) write(level, event string, kv ...string) {
	if l == nil || l.f == nil {
		return
	}
	var b strings.Builder
	b.WriteString(time.Now().Format(time.RFC3339))
	b.WriteByte(' ')
	b.WriteString(level)
	b.WriteByte(' ')
	b.WriteString(event)
	for i := 0; i+1 < len(kv); i += 2 {
		b.WriteByte(' ')
		b.WriteString(kv[i])
		b.WriteByte('=')
		b.WriteString(quoteIfNeeded(kv[i+1]))
	}
	b.WriteByte('\n')
	_, _ = l.f.WriteString(b.String())
}

func quoteIfNeeded(v string) string {
	if v == "" || strings.ContainsAny(v, " \t=") {
		return strconv.Quote(v)
	}
	return v
}
