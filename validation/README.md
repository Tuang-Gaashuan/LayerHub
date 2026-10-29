# v0.1.2-beta validation / 验证范围

2026-10-28 · [Release notes](../RELEASE_NOTES.md)

Release checks include 127 frontend/language/Studio configuration and LF/CRLF generator regressions, Go tests/vet, Windows/Linux builds and actual Windows binaries in disposable offline workspaces. Packaging checks compare binaries, every web asset, documentation, nested source offers and SHA256SUMS. The Update Kit builds its exact shipped sources for both targets. These automated release checks do not contact printers, send mail, import a Windows registry handler or claim a real desktop Studio launch.

Separate attended evidence: X2D homing and positive-direction small steps were individually confirmed onsite, without claiming measured millimetres or every motion condition. A1 light on/off was confirmed onsite and cloud control acceptance observed; A1 movement and a complete print/collection/usage settlement acceptance cycle remain unverified. Historical private evidence and credentials are excluded. Independent deployments must validate their own firmware and network.

中文：本版检查包括 127 项界面/语言/配置生成及换行回归、Go 测试与 vet、Windows/Linux 构建、临时离线目录的实际程序检查和附件内容/校验和核对。更新工具使用交付源码验证两平台构建。这些自动检查不操作打印机、发邮件、代导入注册表或声称 Studio 桌面启动已经验证。X2D 运动和 A1 灯光的现场证据范围分别见对应文档；A1 完整收尾闭环仍待验收。

## Historical v0.1.1-beta record / 历史记录

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
