package app

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"change-timezone/internal/ipq"
	"change-timezone/internal/logx"
	"change-timezone/internal/proxyd"
	"change-timezone/internal/store"
)

type fakeProxy struct {
	info  proxyd.Info
	err   error
	calls int
}

func (f *fakeProxy) Detect() (proxyd.Info, error) { f.calls++; return f.info, f.err }

type fakeQuery struct {
	result  ipq.Result
	err     error
	gotAddr string
	calls   int
}

func (f *fakeQuery) Query(rawURL, proxyAddr string, timeout time.Duration) (ipq.Result, error) {
	f.calls++
	f.gotAddr = proxyAddr
	return f.result, f.err
}

type fakeTZ struct {
	cur          string
	setErr       error
	setCalls     []string
	verifyActual string
	verifyOK     bool
	elevated     bool
	invalid      map[string]bool
}

func (f *fakeTZ) Current() (string, error) { return f.cur, nil }
func (f *fakeTZ) Validate(iana string) error {
	if f.invalid != nil && f.invalid[iana] {
		return errors.New("无效时区: " + iana)
	}
	return nil
}
func (f *fakeTZ) Set(iana string) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.setCalls = append(f.setCalls, iana)
	return nil
}
func (f *fakeTZ) Verify(iana string) (string, bool, error) { return f.verifyActual, f.verifyOK, nil }
func (f *fakeTZ) NeedElevation() bool                      { return f.elevated }
func (f *fakeTZ) SameZone(a, b string) bool                { return a == b }

func newTestEnv(t *testing.T, p *fakeProxy, q *fakeQuery, z *fakeTZ) (Env, *bytes.Buffer, *bytes.Buffer, string) {
	t.Helper()
	dir := t.TempDir()
	var out, errb bytes.Buffer
	env := Env{Proxy: p.Detect, Query: q.Query, TZ: z, DataDir: dir, Log: logx.Nop(), Stdout: &out, Stderr: &errb}
	return env, &out, &errb, dir
}

func fixtures() (*fakeProxy, *fakeQuery, *fakeTZ) {
	return &fakeProxy{info: proxyd.Info{Enabled: true, Addr: "http://127.0.0.1:7890", Kind: "http"}},
		&fakeQuery{result: ipq.Result{IP: "1.2.3.4", Country: "United States", TimezoneID: "America/New_York"}},
		&fakeTZ{cur: "Asia/Shanghai", verifyOK: true, verifyActual: "America/New_York"}
}

func backupFiles(t *testing.T, dir string) []string {
	t.Helper()
	names, err := store.ListBackups(filepath.Join(dir, "backups"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	return names
}

func TestApplyHappyPath(t *testing.T) {
	p, q, z := fixtures()
	env, out, _, dir := newTestEnv(t, p, q, z)
	if code := env.Apply(false, false); code != ExitOK {
		t.Fatalf("退出码 %d, 输出:\n%s", code, out.String())
	}
	files := backupFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("应有 1 份备份: %v", files)
	}
	tzName, _ := store.ReadBackup(filepath.Join(dir, "backups", files[0]))
	if tzName != "Asia/Shanghai" {
		t.Fatalf("备份内容应为当前时区: %q", tzName)
	}
	st, _ := store.LoadState(dir)
	if st.LastApplied != "America/New_York" {
		t.Fatalf("状态应更新: %+v", st)
	}
	if len(z.setCalls) != 1 || z.setCalls[0] != "America/New_York" {
		t.Fatalf("应设置目标时区: %v", z.setCalls)
	}
	if q.gotAddr != "http://127.0.0.1:7890" {
		t.Fatalf("查询应使用代理地址: %q", q.gotAddr)
	}
	if !strings.Contains(out.String(), "[5/5] 修改系统时区……成功（Asia/Shanghai → America/New_York）") {
		t.Fatalf("输出缺少成功文案:\n%s", out.String())
	}
}

func TestApplyNoProxy(t *testing.T) {
	p, q, z := fixtures()
	p.info = proxyd.Info{}
	env, _, errb, dir := newTestEnv(t, p, q, z)
	if code := env.Apply(false, false); code != ExitNoProxy {
		t.Fatalf("退出码 %d", code)
	}
	if !strings.Contains(errb.String(), "未检测到系统代理") || !strings.Contains(errb.String(), "--force") {
		t.Fatalf("错误提示不完整:\n%s", errb.String())
	}
	if n := backupFiles(t, dir); len(n) != 0 {
		t.Fatalf("不应产生备份: %v", n)
	}
	if q.calls != 0 {
		t.Fatal("不应发起查询")
	}
}

func TestApplyPACOnly(t *testing.T) {
	p, q, z := fixtures()
	p.info = proxyd.Info{PACOnly: true}
	env, _, errb, _ := newTestEnv(t, p, q, z)
	if code := env.Apply(false, false); code != ExitPACOnly {
		t.Fatalf("退出码 %d", code)
	}
	if !strings.Contains(errb.String(), "PAC") {
		t.Fatalf("错误提示不完整:\n%s", errb.String())
	}
}

func TestApplyIdempotent(t *testing.T) {
	p, q, z := fixtures()
	z.cur = "America/New_York" // 与目标一致
	env, out, _, dir := newTestEnv(t, p, q, z)
	if code := env.Apply(false, false); code != ExitOK {
		t.Fatalf("退出码 %d", code)
	}
	if n := backupFiles(t, dir); len(n) != 0 {
		t.Fatalf("幂等场景不应备份: %v", n)
	}
	if len(z.setCalls) != 0 {
		t.Fatal("幂等场景不应修改时区")
	}
	if !strings.Contains(out.String(), "已是目标时区，无需修改") {
		t.Fatalf("输出缺少幂等文案:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "state.json")); !os.IsNotExist(err) {
		t.Fatal("幂等场景不应更新状态")
	}
}

func TestApplyNoBackupWhenProgramSet(t *testing.T) {
	p, q, z := fixtures()
	q.result.TimezoneID = "America/New_York"
	z.cur = "Europe/London" // 当前时区是本程序上次设置的
	env, out, _, dir := newTestEnv(t, p, q, z)
	if err := store.SaveState(dir, store.State{LastApplied: "Europe/London"}); err != nil {
		t.Fatal(err)
	}
	if code := env.Apply(false, false); code != ExitOK {
		t.Fatalf("退出码 %d", code)
	}
	if n := backupFiles(t, dir); len(n) != 0 {
		t.Fatalf("防污染：不应产生新备份: %v", n)
	}
	if !strings.Contains(out.String(), "无需备份") {
		t.Fatalf("输出缺少跳过备份文案:\n%s", out.String())
	}
	st, _ := store.LoadState(dir)
	if st.LastApplied != "America/New_York" {
		t.Fatalf("状态应更新: %+v", st)
	}
}

func TestApplyDryRun(t *testing.T) {
	p, q, z := fixtures()
	env, out, _, dir := newTestEnv(t, p, q, z)
	if code := env.Apply(false, true); code != ExitOK {
		t.Fatalf("退出码 %d", code)
	}
	if n := backupFiles(t, dir); len(n) != 0 {
		t.Fatalf("dry-run 不应写备份: %v", n)
	}
	if _, err := os.Stat(filepath.Join(dir, "state.json")); !os.IsNotExist(err) {
		t.Fatal("dry-run 不应写状态")
	}
	if len(z.setCalls) != 0 {
		t.Fatal("dry-run 不应修改时区")
	}
	if !strings.Contains(out.String(), "[DRY-RUN]") || !strings.Contains(out.String(), "仅演示") {
		t.Fatalf("输出缺少 dry-run 标识:\n%s", out.String())
	}
}

func TestApplyQueryFailRollsBackBackup(t *testing.T) {
	p, q, z := fixtures()
	q.err = &ipq.QueryError{Reason: "请求失败: timeout"}
	env, _, errb, dir := newTestEnv(t, p, q, z)
	if code := env.Apply(false, false); code != ExitQueryFailed {
		t.Fatalf("退出码 %d", code)
	}
	if !strings.Contains(errb.String(), "查询出口 IP 失败") {
		t.Fatalf("错误提示不正确:\n%s", errb.String())
	}
	if n := backupFiles(t, dir); len(n) != 0 {
		t.Fatalf("失败应回滚备份: %v", n)
	}
}

func TestApplyDataError(t *testing.T) {
	p, q, z := fixtures()
	q.err = &ipq.DataError{Reason: "timezone.id 为空或格式非法"}
	env, _, _, dir := newTestEnv(t, p, q, z)
	if code := env.Apply(false, false); code != ExitBadTimezone {
		t.Fatalf("退出码 %d", code)
	}
	if n := backupFiles(t, dir); len(n) != 0 {
		t.Fatalf("失败应回滚备份: %v", n)
	}
}

func TestApplyValidateFail(t *testing.T) {
	p, q, z := fixtures()
	z.invalid = map[string]bool{"America/New_York": true}
	env, _, errb, dir := newTestEnv(t, p, q, z)
	if code := env.Apply(false, false); code != ExitBadTimezone {
		t.Fatalf("退出码 %d", code)
	}
	if !strings.Contains(errb.String(), "目标时区校验失败") {
		t.Fatalf("错误提示不正确:\n%s", errb.String())
	}
	if n := backupFiles(t, dir); len(n) != 0 {
		t.Fatalf("失败应回滚备份: %v", n)
	}
}

func TestApplyNeedElevation(t *testing.T) {
	p, q, z := fixtures()
	z.elevated = true
	env, _, errb, dir := newTestEnv(t, p, q, z)
	if code := env.Apply(false, false); code != ExitNoPermission {
		t.Fatalf("退出码 %d", code)
	}
	if !strings.Contains(errb.String(), "sudo") {
		t.Fatalf("错误提示应包含 sudo:\n%s", errb.String())
	}
	if n := backupFiles(t, dir); len(n) != 0 {
		t.Fatalf("权限失败应回滚备份: %v", n)
	}
	if len(z.setCalls) != 0 {
		t.Fatal("不应修改时区")
	}
}

func TestApplySetFail(t *testing.T) {
	p, q, z := fixtures()
	z.setErr = errors.New("tzutil 执行失败")
	env, _, _, dir := newTestEnv(t, p, q, z)
	if code := env.Apply(false, false); code != ExitSetFailed {
		t.Fatalf("退出码 %d", code)
	}
	if n := backupFiles(t, dir); len(n) != 0 {
		t.Fatalf("修改命令失败应回滚备份: %v", n)
	}
}

func TestApplyVerifyMismatchKeepsBackup(t *testing.T) {
	p, q, z := fixtures()
	z.verifyOK = false
	z.verifyActual = "Asia/Shanghai"
	env, _, errb, dir := newTestEnv(t, p, q, z)
	if code := env.Apply(false, false); code != ExitSetFailed {
		t.Fatalf("退出码 %d", code)
	}
	if !strings.Contains(errb.String(), "校验未通过") {
		t.Fatalf("错误提示不正确:\n%s", errb.String())
	}
	if n := backupFiles(t, dir); len(n) != 1 {
		t.Fatalf("已尝试修改，备份应保留: %v", n)
	}
}

func TestApplyForceSkipsDetection(t *testing.T) {
	p, q, z := fixtures()
	p.err = errors.New("检测命令不可用") // force 模式下检测失败不致命
	env, out, _, _ := newTestEnv(t, p, q, z)
	if code := env.Apply(true, false); code != ExitOK {
		t.Fatalf("退出码 %d, 输出:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "跳过（--force）") {
		t.Fatalf("输出应包含 force 跳过说明:\n%s", out.String())
	}
}

func TestApplyBackupCleanup(t *testing.T) {
	p, q, z := fixtures()
	env, _, _, dir := newTestEnv(t, p, q, z)
	// 预置 25 份旧备份后触发一次成功写入
	bdir := filepath.Join(dir, "backups")
	if err := os.MkdirAll(bdir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 25; i++ {
		name := "20250101-0000" + string(rune('a'+i)) + "-00-Old_Tz"
		if err := os.WriteFile(filepath.Join(bdir, name), []byte("Old/Tz\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if code := env.Apply(false, false); code != ExitOK {
		t.Fatalf("退出码 %d", code)
	}
	if n := backupFiles(t, dir); len(n) != 20 {
		t.Fatalf("清理后应保留 20 份: %d", len(n))
	}
}

func TestRestoreNoBackups(t *testing.T) {
	p, q, z := fixtures()
	env, _, errb, _ := newTestEnv(t, p, q, z)
	if code := env.Restore("", false); code != ExitBackupFailed {
		t.Fatalf("退出码 %d", code)
	}
	if !strings.Contains(errb.String(), "暂无可恢复的备份") {
		t.Fatalf("错误提示不正确:\n%s", errb.String())
	}
}

func TestRestoreNamedMissing(t *testing.T) {
	p, q, z := fixtures()
	env, _, errb, dir := newTestEnv(t, p, q, z)
	if _, err := store.WriteBackup(filepath.Join(dir, "backups"), "Asia/Shanghai", time.Now()); err != nil {
		t.Fatal(err)
	}
	if code := env.Restore("不存在的备份", false); code != ExitBackupFailed {
		t.Fatalf("退出码 %d", code)
	}
	if !strings.Contains(errb.String(), "指定的备份不存在") {
		t.Fatalf("错误提示不正确:\n%s", errb.String())
	}
}

func TestRestoreHappyPath(t *testing.T) {
	p, q, z := fixtures()
	z.cur = "America/New_York"
	env, out, _, dir := newTestEnv(t, p, q, z)
	path, err := store.WriteBackup(filepath.Join(dir, "backups"), "Asia/Shanghai", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if code := env.Restore("", false); code != ExitOK {
		t.Fatalf("退出码 %d, 输出:\n%s", code, out.String())
	}
	if len(z.setCalls) != 1 || z.setCalls[0] != "Asia/Shanghai" {
		t.Fatalf("应恢复为备份时区: %v", z.setCalls)
	}
	st, _ := store.LoadState(dir)
	if st.LastApplied != "Asia/Shanghai" {
		t.Fatalf("状态应更新: %+v", st)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("恢复后备份文件应保留")
	}
}

func TestRestoreSameZone(t *testing.T) {
	p, q, z := fixtures()
	z.cur = "Asia/Shanghai" // 已是备份的时区
	env, out, _, dir := newTestEnv(t, p, q, z)
	if _, err := store.WriteBackup(filepath.Join(dir, "backups"), "Asia/Shanghai", time.Now()); err != nil {
		t.Fatal(err)
	}
	if code := env.Restore("", false); code != ExitOK {
		t.Fatalf("退出码 %d", code)
	}
	if len(z.setCalls) != 0 {
		t.Fatal("同区不应修改")
	}
	if !strings.Contains(out.String(), "已是该时区") {
		t.Fatalf("输出不正确:\n%s", out.String())
	}
	st, _ := store.LoadState(dir)
	if st.LastApplied != "Asia/Shanghai" {
		t.Fatalf("状态应更新: %+v", st)
	}
}

func TestRestoreDryRun(t *testing.T) {
	p, q, z := fixtures()
	z.cur = "America/New_York"
	env, out, _, dir := newTestEnv(t, p, q, z)
	if _, err := store.WriteBackup(filepath.Join(dir, "backups"), "Asia/Shanghai", time.Now()); err != nil {
		t.Fatal(err)
	}
	if code := env.Restore("", true); code != ExitOK {
		t.Fatalf("退出码 %d", code)
	}
	if len(z.setCalls) != 0 {
		t.Fatal("dry-run 不应修改")
	}
	if _, err := os.Stat(filepath.Join(dir, "state.json")); !os.IsNotExist(err) {
		t.Fatal("dry-run 不应写状态")
	}
	if !strings.Contains(out.String(), "仅演示") {
		t.Fatalf("输出缺少 dry-run 标识:\n%s", out.String())
	}
}

func TestRestoreNeedElevation(t *testing.T) {
	p, q, z := fixtures()
	z.cur = "America/New_York"
	z.elevated = true
	env, _, _, dir := newTestEnv(t, p, q, z)
	if _, err := store.WriteBackup(filepath.Join(dir, "backups"), "Asia/Shanghai", time.Now()); err != nil {
		t.Fatal(err)
	}
	if code := env.Restore("", false); code != ExitNoPermission {
		t.Fatalf("退出码 %d", code)
	}
}

func TestList(t *testing.T) {
	p, q, z := fixtures()
	env, out, _, dir := newTestEnv(t, p, q, z)
	if code := env.List(); code != ExitOK || !strings.Contains(out.String(), "暂无备份") {
		t.Fatalf("空目录: 退出码 %d, 输出:\n%s", code, out.String())
	}
	out.Reset()
	if _, err := store.WriteBackup(filepath.Join(dir, "backups"), "Asia/Shanghai", time.Now()); err != nil {
		t.Fatal(err)
	}
	if code := env.List(); code != ExitOK || !strings.Contains(out.String(), "Asia/Shanghai") {
		t.Fatalf("有备份: 退出码 %d, 输出:\n%s", code, out.String())
	}
}
