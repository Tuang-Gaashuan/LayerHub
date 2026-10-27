# LayerHub v0.1.1-beta · Controls, updates and multi-plate intake

**Recommended: LayerHub Beta + Bambu Studio.** This release bundles the previously local work into one update:

- Faster control feedback and stable telemetry refreshes that preserve console tabs, focus and camera.
- Printer-side stop reconciliation and manual collection/usage settlement. Interrupted jobs are not successful prints; a settled stopped job no longer incorrectly blocks the next job.
- Explicit sliced-3MF plate selection, safe PNG previews, per-plate material/color and estimates, and atomic queueing of up to 16 single-filament plates. Stable receipt IDs prevent duplicate batches after uncertain responses; shared source files remain until their last job is deleted.
- Plate IDs throughout task lists, control, preparation and final confirmation. Queueing never automatically starts or chains prints.
- A1 without AMS preparation and bilingual LAN/Developer Mode guidance. A1 physical validation is pending.
- A standalone Windows/Linux Download / Update Kit, including verified downloads, channel selection, offline transfer, idle checks, backups and coordinated updates to an existing NAS/cloud deployment.

Download the Windows ZIP or Linux amd64 TAR.GZ application and **SHA256SUMS**. The separate **LayerHub-Download-UpdateKit-0.1.1-beta.zip** supplies the download/update entrypoints. Linux updates require Python 3 and native Debian/Ubuntu systemd; initial network/certificate/tunnel setup, Docker and ARM are outside this updater's scope.

**Back up the full workspace and master key before upgrading.** Update NAS backend and cloud UI together while idle. Database schema advances to **5**; reverting to an older reader requires the matching pre-upgrade workspace backup and must account for newer jobs/settlements. The kit never blindly replaces data after a new reader starts.

Go tests/vet, generated translations, 94 frontend/language tests, Windows/Linux builds and offline Windows binary checks passed. Desktop Chinese/English and mobile Chinese dialogs were checked. These tests issue no real print or email. A1 and real Studio multi-plate printing remain attended acceptance work; no new multicolor or dual-nozzle start compatibility is claimed. Publishing does not deploy the existing production service.

[Deployment](https://github.com/Tuang-Gaashuan/LayerHub/blob/v0.1.1-beta/DEPLOYMENT.md) · [Multi-plate guide](https://github.com/Tuang-Gaashuan/LayerHub/blob/v0.1.1-beta/MULTIPLATE.md) · [A1 setup](https://github.com/Tuang-Gaashuan/LayerHub/blob/v0.1.1-beta/A1_SETUP.md) · [Update Kit](https://github.com/Tuang-Gaashuan/LayerHub/blob/v0.1.1-beta/deploy/update-kit/README.md)

## 中文

本次将控制响应与稳定刷新、机身终止后的收尾与空闲判定、多盘明确选择和原子入队、断网防重复、共用文件保护、A1 无 AMS 准备及一键下载/更新工具统一发布。最多安排 16 个单耗材盘，盘号贯穿任务、准备和确认；不自动连续开打，中断后仍需人工取件并结算实际用量。

下载对应系统应用和 SHA256SUMS；另有 **LayerHub-Download-UpdateKit-0.1.1-beta.zip**。升级前备份工作空间和主密钥，空闲时统一更新 NAS 与云端。数据库升级到 **schema 5**，回退旧程序须配套恢复升级前备份，不能直接读新库或覆盖新任务和结算。Linux 更新需 Python 3，首次网络/证书/隧道仍按部署指导配置。

Go、翻译生成、94 项前端/语言测试、跨平台构建和真实 Windows 程序离线验收通过。A1 和真实多盘逐盘启动仍待现场验收，不宣称新增多色/双喷嘴启动支持。本次上传不部署生产环境。Beta 继续配合 Bambu Studio；Alpha 工作台分支和历史标签保留。

[中文部署](https://github.com/Tuang-Gaashuan/LayerHub/blob/v0.1.1-beta/README.zh-CN.md) · [多盘导入](https://github.com/Tuang-Gaashuan/LayerHub/blob/v0.1.1-beta/MULTIPLATE.md) · [A1 接入](https://github.com/Tuang-Gaashuan/LayerHub/blob/v0.1.1-beta/A1_SETUP.md) · [工具操作](https://github.com/Tuang-Gaashuan/LayerHub/blob/v0.1.1-beta/deploy/update-kit/README.zh-CN.md)

---

## Previous v0.1.0-beta baseline / 上一版本基线

Beta is the ongoing mainline. Prepare and slice in Bambu Studio; use LayerHub for teams, printers, per-device queues, physical spools, remote control and notifications.

- Remove model workbench, geometry/STL import, online slicing, process presets and shared model projects, including their frontend assets, Three.js and backend/engine runtime.
- Accept only actual sliced 3MF; reject raw G-code, STL and unsliced projects in both UI and backend.
- Inspect files directly into a compact dashboard card, with colors, plate estimates and references. No import-purpose dialog remains.
- Preserve accounts, roles, queues, inventory, controls, camera, email, diagnostics and backups. Existing alpha project rows remain in database backups; fresh Beta does not create editor tables.
- Retire old model/project APIs with HTTP 410; send old workbench bookmarks to the dashboard. NAS engine configuration is no longer required or read.
- Ship Windows/Linux amd64, corresponding AGPL source, bilingual guidance and an Alpha → Beta migration guide as one release.

以后以 Beta 持续更新，首版 v0.1.0-beta。模型准备与切片交给 Bambu Studio，LayerHub 专注团队、打印机、队列、耗材、控制与通知。移除完整工作台前后端及引擎代码，只接收已切片 3MF，直接校验后安排任务；账号与历史业务保留，旧工程数据随数据库备份保留。云端—NAS—打印机部署及中英文文档同步精简。

The numbering resets for this new channel; alpha releases remain available for recovery. Read [migration](BETA_MIGRATION.md) before updating an existing installation. No new hardware, multicolor or dual-nozzle compatibility is claimed. Docker runtime and a fresh two-host OS installation still require independent acceptance.
