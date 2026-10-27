# v0.1.1-beta validation / 验证范围

2026-10-27 · [Release notes](../RELEASE_NOTES.md)

Go tests/vet, generated translation checks and **94 frontend/language tests** pass. Windows/Linux amd64 application and updater builds carry corresponding vendored sources. Temporary offline Windows workspaces exercise fresh startup/restart, diagnostics, sliced-only intake and multi-plate replay/reservations/file lifetime. No real printer or test email is used.

Regression covers printer-side stops and settled readiness, atomic batch rollback, user-scoped receipts, sparse plate IDs, safe PNG previews, A1 external-spool preparation, schema migration/refusal, and backup/restore. Browser checks cover desktop Chinese/English and mobile Chinese batch/spool dialogs. The [earlier A1/multi-plate record](a1-multiplate-preview-20261027.md) records pre-publication validation.

A1 hardware and real Studio multi-plate printing still require attended acceptance. Docker, ARM and fresh two-host OS installation remain unverified. Publishing does not deploy production; previous attended X2D evidence does not prove broader hardware, multicolor or dual-nozzle compatibility.

中文：94 项前端/语言及 Go 检查通过，实际程序在临时离线目录验收，不操作生产打印机或发送邮件。A1、真实多盘逐盘打印及新平台部署仍须独立验收，上传 GitHub 不代表生产已升级。以下保留上一版的历史验证记录。

## Historical Beta 0.1.0 validation / 上版记录

0.1.0-beta · 2026-10-27

This release removes the editor/engine and focuses on team printer management. The current record is `beta-release.json`; historical Alpha evidence remains in the immutable Alpha release tags rather than the Beta package.

- 83 frontend/language tests cover team/security/controls/queues/materials/notifications, removal of workbench navigation, sliced-only intake, inline reports and estimates, cancellation and bilingual messages.
- Go tests cover retired API denial, actual sliced-content validation, unsafe archive rejection, temporary cleanup, unchanged business state and preservation/backup of legacy private rows, alongside existing device/account/mail/inventory tests. Go vet and Windows/Linux amd64 builds are part of release checks.
- Portable fresh-workspace, doctor and file-intake scripts exercise actual binaries in disposable offline directories. No physical print, printer command or SMTP delivery is required for these tests.
- The guarded production migration backs up before switching, requires fresh idle/finished telemetry and no active tasks/uncertain control, and verifies retained accounts/inventory/jobs and disabled editor capabilities. Results are recorded after deployment.
- Docker runtime and a fresh two-host OS installation remain unverified. Existing attended X2D print evidence does not establish broader printer/firmware/multicolor/dual-nozzle support.

本版围绕团队打印管理，验证工作台移除、切片 3MF 白名单、界面简化、任务/库存/通知和账号权限。83 项前端/语言测试通过；Go 测试核对旧接口停用、文件校验、临时清理、无业务副作用，以及旧 Alpha 私有数据的保留和备份。实际程序检查使用独立离线目录，不自动实机打印或发送测试邮件。生产迁移先备份并核对空闲状态，具体结果见本版 JSON；旧 Alpha 验证材料保留在历史标签中。
