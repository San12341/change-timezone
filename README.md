# change-timezone

根据代理出口 IP 归属地自动切换系统时区的命令行工具（Windows / macOS），零第三方依赖的 Go 单二进制。

详细需求与设计决策见 [REQUIREMENTS.md](REQUIREMENTS.md)。

## 用法

```
change-timezone apply  [--dry-run] [--force]     检测代理并切换时区
change-timezone restore [备份文件名] [--dry-run]  恢复备份的时区（默认最新一份）
change-timezone list                             列出全部备份
change-timezone version                          显示版本
```

- `apply`：检测系统代理（未开启则退出码 2 结束）→ 备份当前时区 → 通过代理查询 ipwho.is 出口 IP 归属时区 → 修改系统时区并回读校验。
- `--force`：跳过代理开关检测（TUN 模式用户专用）。
- `--dry-run`：完整执行检测/查询/匹配并展示结果，但不做任何系统改动。
- Windows 需要管理员权限（内置 UAC manifest，双击自动弹提权）；macOS 修改时区时需 `sudo` 手动运行。

## 构建

```
make build   # 运行 go vet + go test，交叉编译 darwin/arm64、darwin/amd64、windows/amd64、windows/arm64 到 dist/
```

首次构建 Windows 目标会通过 `go run github.com/akavel/rsrc@v0.10.2` 生成 UAC manifest 资源（构建期工具，`go.mod` 保持零依赖）。

版本号在构建时由 git tag 注入（`v1.2.3` → `1.2.3`，未打 tag 时回退为提交哈希，非 git 环境为 `dev`），也可显式指定：`make build VERSION=1.2.3`。

## 数据文件

```
~/.change-timezone/
├── backups/              # 时区备份（保留最近 20 份）
├── state.json            # 防备份污染的状态（记录上次由本程序设置的时区）
└── change-timezone.log   # 运行日志（超过 1 MB 轮转）
```

## 退出码

| 码 | 含义 |
|---|---|
| 0 | 成功（含幂等跳过、dry-run） |
| 1 | 未知错误 |
| 2 | 未检测到系统代理 |
| 3 | IP 查询失败 |
| 4 | 权限不足 |
| 5 | 时区数据异常 |
| 6 | 备份/文件操作失败 |
| 7 | 时区修改失败 |
| 8 | PAC 模式限制 |
| 64 | 用法错误 |
