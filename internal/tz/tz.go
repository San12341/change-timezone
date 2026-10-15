// Package tz 封装系统时区的读取、校验与修改。
// RealOps 通过真实系统命令实现各平台行为（darwin/windows 文件内为平台实现）。
package tz

// RealOps 是 TZOps 的真实实现。
type RealOps struct{}

func (RealOps) Current() (string, error)                 { return current() }
func (RealOps) Validate(iana string) error               { return validate(iana) }
func (RealOps) Set(iana string) error                    { return set(iana) }
func (RealOps) Verify(iana string) (string, bool, error) { return verify(iana) }
func (RealOps) NeedElevation() bool                      { return needElevation() }
func (RealOps) SameZone(a, b string) bool                { return sameZone(a, b) }
