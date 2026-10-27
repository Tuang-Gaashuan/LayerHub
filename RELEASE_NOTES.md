# LayerHub v0.1.0-beta · Team printer management / 团队打印机管理

Beta is the ongoing mainline. Prepare and slice in Bambu Studio; use LayerHub for teams, printers, per-device queues, physical spools, remote control and notifications.

- Remove model workbench, geometry/STL import, online slicing, process presets and shared model projects, including their frontend assets, Three.js and backend/engine runtime.
- Accept only actual sliced 3MF; reject raw G-code, STL and unsliced projects in both UI and backend.
- Inspect files directly into a compact dashboard card, with colors, plate estimates and references. No import-purpose dialog remains.
- Preserve accounts, roles, queues, inventory, controls, camera, email, diagnostics and backups. Existing alpha project rows remain in database backups; fresh Beta does not create editor tables.
- Retire old model/project APIs with HTTP 410; send old workbench bookmarks to the dashboard. NAS engine configuration is no longer required or read.
- Ship Windows/Linux amd64, corresponding AGPL source, bilingual guidance and an Alpha → Beta migration guide as one release.

以后以 Beta 持续更新，首版 v0.1.0-beta。模型准备与切片交给 Bambu Studio，LayerHub 专注团队、打印机、队列、耗材、控制与通知。移除完整工作台前后端及引擎代码，只接收已切片 3MF，直接校验后安排任务；账号与历史业务保留，旧工程数据随数据库备份保留。云端—NAS—打印机部署及中英文文档同步精简。

The numbering resets for this new channel; alpha releases remain available for recovery. Read [migration](BETA_MIGRATION.md) before updating an existing installation. No new hardware, multicolor or dual-nozzle compatibility is claimed. Docker runtime and a fresh two-host OS installation still require independent acceptance.
