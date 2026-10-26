# Release validation / 发布验证

0.3.0-alpha · 2026-10-27. Tests used the exported source or binaries rebuilt from it. Disposable release tests used no live workspace, printer commands or SMTP deliveries. The separately guarded production rollout is recorded in diagnostics-release.json; it backs up existing data and preserves account preferences, tasks and print settings.

| Check | Result / scope |
|---|---|
| Exported Go source | `go test -mod=vendor ./...` and `go vet -mod=vendor ./...` passed on Windows; Windows/Linux amd64 builds passed |
| Frontend, geometry and language | 183 tests passed, zero failed |
| Windows fresh workspace | Passed: offline and print-start off, random owner credentials, private-file denial, source offer and restart persistence |
| Linux fresh workspace | Same checks passed as an unprivileged user |
| HTTPS and SSH gateway (0.3.0 diagnostics update) | Passed on isolated temporary listeners on one Linux host: template syntax/effective restrictions, allowed reverse forward, disallowed alternate port and Shell, verified TLS, Secure cookie, public internal-path denial and 503 after tunnel loss while the independent gateway marker stays available |
| Portable launchers | Windows command quoting/control characters checked; Linux shell syntax checked |
| Docker | YAML/defaults/volume/permissions checked statically; image build and container startup **not executed** because no Docker engine was available |
| Fresh OS installation | systemd installer is fresh-only; a clean two-host OS installation and every optional slicer GUI-library combination were **not executed** in this release check |
| Publication hygiene | Whitelist export; production host/address/code, personal paths, private keys/tokens and runtime-data checks; see publication-scan.json |

Language validation covers browser selection, English forms and control payloads, protected user content, draft-before-reload handling, per-user preferences, English server errors and locally generated English mail with milestone deduplication. Real browser checks covered English sign-in/workbench and narrow-screen overflow. No real printer control or SMTP delivery was performed for these diagnostics checks.

JSON reports in this folder contain no credentials or recipient addresses. Historical X2D real-print evidence describes an existing tested configuration; it is not a promise that every catalog model, firmware or newly installed slicing engine will work. Follow [independent acceptance tests](../TESTING.md) with your own hardware.

## 简体中文

本次使用导出的源码和重新构建的程序验证。Go 测试与 vet、183 项前端/几何测试、Windows/Linux 全新工作区与重启检查均通过。网关使用同一 Linux 主机上的独立临时监听器，验证了受限隧道、HTTPS、禁止 Shell/其他转发端口、私有路径拦截和断线 503；不代表已经替别人完成全新双主机部署。

隔离测试未使用生产工作区，未发送真实打印控制或 SMTP 邮件。另行执行的生产更新记录在 i18n-release.json：部署前备份并检查空闲状态，保留账号、任务和打印开关。本版重新执行隔离网关验证，并验证隧道断开后独立网关标记仍可用。Docker 仅静态检查；首次系统安装器和所有切片图形依赖组合仍需外部测试者按验收表实际验证。机型图鉴不等于控制兼容列表。

Diagnostic acceptance / 诊断验收：local inspection does not initialize data, load drivers, bind a listener or open SQLite; opt-in temp-file probes leave business files unchanged. Runtime checks require current administrator access, separate fresh telemetry from authorization, reject simulated/future/stale evidence, and omit private data. Browser tests cover gateway-only success, NAS failure, old-response isolation, role loss, stable DOM and report expiry. The gateway template marker is independent of the NAS.
