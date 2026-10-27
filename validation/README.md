# Beta validation / Beta 验证

0.1.0-beta · 2026-10-27

This release removes the editor/engine and focuses on team printer management. The current record is `beta-release.json`; historical Alpha evidence remains in the immutable Alpha release tags rather than the Beta package.

- 83 frontend/language tests cover team/security/controls/queues/materials/notifications, removal of workbench navigation, sliced-only intake, inline reports and estimates, cancellation and bilingual messages.
- Go tests cover retired API denial, actual sliced-content validation, unsafe archive rejection, temporary cleanup, unchanged business state and preservation/backup of legacy private rows, alongside existing device/account/mail/inventory tests. Go vet and Windows/Linux amd64 builds are part of release checks.
- Portable fresh-workspace, doctor and file-intake scripts exercise actual binaries in disposable offline directories. No physical print, printer command or SMTP delivery is required for these tests.
- The guarded production migration backs up before switching, requires fresh idle/finished telemetry and no active tasks/uncertain control, and verifies retained accounts/inventory/jobs and disabled editor capabilities. Results are recorded after deployment.
- Docker runtime and a fresh two-host OS installation remain unverified. Existing attended X2D print evidence does not establish broader printer/firmware/multicolor/dual-nozzle support.

本版围绕团队打印管理，验证工作台移除、切片 3MF 白名单、界面简化、任务/库存/通知和账号权限。83 项前端/语言测试通过；Go 测试核对旧接口停用、文件校验、临时清理、无业务副作用，以及旧 Alpha 私有数据的保留和备份。实际程序检查使用独立离线目录，不自动实机打印或发送测试邮件。生产迁移先备份并核对空闲状态，具体结果见本版 JSON；旧 Alpha 验证材料保留在历史标签中。
