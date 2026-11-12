# LayerHub v0.1.4-beta · Inventory history and result recovery

[简体中文使用说明](BETA_014.zh-CN.md) · [English guide](BETA_014.md)

## 简体中文

- **耗材操作流水**：记录登记、修改、挂载卸载及用量结算的操作者、时间、前后余量和位置，与库存事务一起保存。升级时为现有库存建立起点，不补造过去的历史。
- **统一完工处理**：集中处理待取件、待结算及缺失任务补登记。打开页面不会自动取件、扣料或启动下一盘。
- **丢失响应后的恢复**：库存和正常结算操作带账户隔离的服务端回执，重连后只读核对，未查到仍保持未知。可以在自行核对最新记录后清除本地提示；这不确认操作成功，也不解除设备控制核对锁。
- 合并近期库存卷复用、取消色号差异提醒、耗材操作保留控制台上下文、检修暂停汇报、双喷嘴文件导入及库存版本保护。
- Windows/Linux AMD64 应用和下载更新工具统一升级，中英文文档及对应源码随包提供。ARM64 保持初步验证说明，不宣称本版已完成真实打印、摄像头或长期生产验收。

升级前备份数据库和密钥；保持同一版本程序与数据配套，不盲目恢复旧数据库。设备控制仍保留独立的回执和人工核对，服务端保存回执不等于设备执行成功。

## English

- **Spool activity** records actors, time, before/after stock, mount changes and usage settlement atomically with inventory. Existing stock receives an activation baseline; earlier history is not fabricated.
- **Print completion** groups collection, usage settlement and missing-job reconciliation. Opening the entry never collects a print, consumes stock or starts another plate.
- **Lost-response recovery** reads account-scoped durable receipts for inventory mutations and normal settlement. Missing receipts remain unknown. Explicit dismissal after checking records clears only a browser notice, not device-review locks or inventory.
- Include recent stock-reuse, color-metadata, console-return, maintenance, dual-nozzle import and stock-revision fixes.
- Versioned Windows/Linux AMD64 apps, Update Kit, bilingual documentation and corresponding source are bundled. ARM64 remains preliminary; do not interpret cross-builds as hardware printing, camera or long-term acceptance.

Back up the database and key before upgrading. Keep matching program/data versions and use reviewed backup recovery. A server save receipt does not establish physical device execution.

[Usage](USAGE.md) · [Deployment](DEPLOYMENT.md) · [Update Kit](deploy/update-kit/README.md) · [Changes](CHANGELOG.md)
