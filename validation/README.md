# Release validation / 发布验证

## 0.4.0-alpha · File intake / 文件导入

The file-workflow batch is delivered as one version. Its new acceptance record is [file-intake-release.json](file-intake-release.json). Historical diagnostic/gateway and real-print records below retain their original scope.

- 189 frontend/geometry/language tests passed. Focused import/inspection tests passed again after modal-width adjustments.
- Full exported-source Go tests and vet passed; Windows/Linux amd64 application and mail-worker builds passed.
- Actual-binary inspection smoke tests cover geometry-only, slice-only and combined archives, digest matching, reference-field allowlisting, unsliced queue rejection and temporary-file cleanup in disposable offline workspaces. Reports: [Windows](file-intake-windows.json), [Linux](file-intake-linux.json).
- Browser checks cover Chinese and English inspection, geometry parameter comparison, local JSON report download, switching from the workbench to sliced-file scheduling and rounded estimates. Screenshots use a disposable demo, never a real print file: [中文](file-intake-zh.png), [English](file-intake-en.png).
- Queue/upload tests use fake devices; no real print-start, temperature/fan/light command, inventory settlement or SMTP test is part of this batch.
- Existing Docker static-only and fresh two-host OS installation limitations remain. This release makes no new printer/firmware, multicolor or dual-nozzle compatibility claim.

本次将内容识别、双入口、参数对照、逐盘颜色与预计值、报告导出及后端队列校验作为一个版本交付。189 项前端/几何/语言测试、完整 Go 测试与 vet、Windows/Linux 构建均通过。实际程序测试使用独立离线工作区；浏览器核对了中英文、报告下载、跨入口和预填值。生产更新与保留数据的结果见本版 JSON；下方诊断/网关和历史实机证据保留原有验证范围。

# Previous validation baseline / 既有验证基线

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
