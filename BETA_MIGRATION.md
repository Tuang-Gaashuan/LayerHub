# Alpha → Beta migration / Alpha → Beta 迁移

## English

`v0.1.0-beta` starts the new long-term mainline. The version number resets deliberately; it is the streamlined successor to `v0.4.0-alpha`, not a return to the original alpha implementation. Most ongoing team/printer feature refinement targets Beta on `main`. Alpha's experimental built-in workbench will continue gradually on the independent `alpha` branch. Version tags and their source/releases remain fixed. See the [channel comparison](CHANNELS.md).

### What is removed

- Model workbench, STL/project geometry import, 3D canvas/editing, process presets, online slicing and shared model snapshots.
- Their frontend assets, Three.js runtime, backend routes and engine/background slicing code. Old model/team-project APIs return 410 and old workbench bookmarks land on the dashboard.
- NAS engine installation/configuration requirements. Legacy `LAYERHUB_SLICER_*` settings are ignored; Beta cannot launch the old engine.

### What is retained

Accounts/roles/sessions, printers and encrypted credentials, individual spool inventory, historical jobs, existing queue files, email preferences/outbox, controls, camera, audit records and owner backup/restore remain. New uploads must be actual sliced 3MF. Existing historical raw-file records are not deleted.

Beta performs no DROP or cleanup of existing private model/project tables or blobs. These retired rows remain in the existing SQLite database and are included in full backups, but are not accessible through Beta model APIs. A fresh Beta workspace does not create those tables. To use old projects, recover an offline copy with the matching alpha release; do not run two programs against one live data directory.

### Upgrade an existing installation

1. Wait for fresh idle/finished devices, no transfer/start/pending or uncertain control, and no active alpha slicing task or pending restore.
2. Export an owner backup and retain a filesystem snapshot of the data directory/master key and existing environment/service configuration. Keep the old binary/web release.
3. Stop the app, install the complete Beta binary and web directory as a new release, and point the existing service at it. Do not copy new files over old web assets, because removed editor files would remain served. Do not use the fresh-only installer on this existing service.
4. Preserve the existing data directory and relevant settings. Remove obsolete slicing environment entries. No engine files or printer commands need to be executed. Previously installed external engine files may remain inactive for rollback.
5. Start once, check the version, sign in and refresh the browser. Verify roles, spool balances, job/queue records, email preferences and live telemetry. Confirm editor assets are absent, old APIs return 410, and sliced 3MF validation works without creating a test print.

For rollback, preserve all new Beta work first and use the matching data snapshot with the previous alpha program in a maintenance window. Never overwrite a live/newer database with an old backup while members are working. Backup restoration remains owner-only, offline and explicit; restored devices are disabled until checked.

## 简体中文

`v0.1.0-beta` 是以后持续维护的新主线，版本号有意从 0.1.0 重新开始，是 `v0.4.0-alpha` 的精简后继版本，不代表退回最初的 Alpha。团队与打印管理功能主要在 `main` 的 Beta 上完善；Alpha 内置工作台在独立 `alpha` 分支缓慢推进。已发布的版本标签、源码与发布包保持不变。详见[版本区别与维护政策](CHANNELS.md#简体中文)。

移除：模型工作台、STL/工程几何导入、三维编辑、工艺预设、在线切片、共享模型快照，以及对应前端文件、Three.js、后端接口和后台切片代码。旧模型/工程接口返回 410，旧工作台书签回到看板。NAS 不再依赖切片引擎；旧 `LAYERHUB_SLICER_*` 配置被忽略，Beta 不会启动引擎。

保留：账号角色与会话、打印机与加密凭据、逐卷库存、历史任务与排队文件、邮箱偏好和待发记录、控制、摄像头、操作记录及所有者备份恢复。新上传仅限有效切片 3MF，已有原始文件历史记录不自动删除。

不会清理已有数据库中的私人模型/工程表及 BLOB，它们仍随完整数据库备份保留，只是不再通过 Beta 模型接口访问。新装 Beta 不创建这些表。需要打开旧工程时，在独立离线副本中使用对应 Alpha 版本；两个程序不能共用同一运行中的数据目录。

升级步骤：先等设备空闲/完工、新鲜遥测、无传输/启动/待核对指令、无 Alpha 活动切片或待恢复；导出所有者备份，并保存数据目录、master.key、环境/服务配置和旧程序。停止应用，把 Beta 程序与完整 web 安装到新的发布目录，再切换服务指向，避免覆盖旧 web 后遗留编辑器文件。保留原数据目录与相关配置，移除旧切片环境项；已安装的外部引擎可保持不运行，供回退使用。只启动一次并核对版本、账号、库存、任务、通知偏好、遥测和切片文件校验，不需要实机试打。

回退前先保留 Beta 期间新增的业务，再在维护窗口使用对应旧程序和数据快照。成员操作期间不得用旧备份覆盖新数据库。备份恢复仍要求所有者明确操作、离线启动，恢复后设备保持停用，等待核对。


## Beta 0.1.0 → 0.1.1

Back up the complete workspace and master key. Upgrade NAS/backend and cloud/UI together while idle. Schema 5 preserves explicit plate selection; pre-feature readers cannot open it. Rollback needs the matching pre-upgrade data backup and loses intervening changes. Never overwrite newly created jobs/settlements with an old snapshot.

升级前备份完整工作空间和主密钥，在空闲时统一更新 NAS 与云端。本版数据库为 schema 5，旧程序不能直接读取；回退须配套恢复升级前数据，不能覆盖升级后的新任务与结算。新程序启动后若出问题，更新工具保留程序、数据和备份供检查，不盲目覆盖回退。
