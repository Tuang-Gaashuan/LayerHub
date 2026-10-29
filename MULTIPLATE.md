# Multi-plate sliced 3MF / 多盘切片文件

v0.1.2-beta, prepared 2026-10-27. Use with Bambu Studio. No model editing or online slicing is introduced.

## 中文

在 Bambu Studio 为实际设备与耗材切片，并导出包含所需打印盘的**已切片 3MF**。LayerHub 不接受 STL、未切片工程或原始 G-code。

1. 在协作看板选择“新建任务”，上传文件。检查通过后展开盘列表，查看盘号、预览、材质、颜色代码、预计时长与用量。
2. 单盘文件自动选中这一盘；多盘文件初始不选任何盘。未选盘时点击“下一步”会打开选盘界面，勾选本次需要的打印盘后继续。只选一盘时显示该盘的颜色、切片净用量，并填入该盘自己的预计时长与向上取整的预留量，盘号会保存到任务中。
3. 勾选多盘后进入逐盘审核，最多 16 盘。选择目标设备，也可暂时不分配；每盘分别选择实际料卷、预留克数与预计分钟数。耗材选择使用颜色卡，同时显示名称、材质、余量及登记的装载位置。
4. 确认后整批加入队列：每盘产生一个独立任务。如果某盘库存不足、材质不同或机型不匹配，整批不保存，不会留下半批任务。共同使用一卷时会合并检查预留总量。
5. 每个任务继续经过打印准备、供料映射及实际启动确认。盘号贯穿导入、任务和准备；改变准备中的盘号会同时更新盘号与有记录的预计时长。保存准备前不能直接传输或启动。

**入队不启动打印，也不代表可以自动连续打印。** 每次打印前仍需现场确认上一件已取走、打印板已清空。真实消耗按后续结算扣减，入队只预留库存。

### 调整打印顺序

在“打印队列”展开任务的“调整顺序”，可上移、下移或“设为下一项”。设为下一项需要再次确认，只改变等待顺序，不发送文件或启动指令。列表标出下一项；非队首任务需要先调序或等待前项结束，不能绕过队首直接开打。

管理员可调整同一台设备的等待任务。普通成员仅能调整自己连续排列的等待任务，不能越过其他成员的任务。已经传输、正在打印或等待核对的任务保持原位。调序不会改变料卷、预留量或已保存的打印准备。

保存时会核对整条设备队列；其他成员新增、移走、传输或调整任务后，旧页面的操作会被拒绝。遇到连接中断或结果不明确，先“重新读取队列”，核对后再操作，系统不会自动重试调序。

### 当前范围

- 批量安排限**单喷嘴、单耗材盘**；多耗材盘可单独导入，再按已适配机型完成映射；多喷嘴启动限制继续保留。
- 保留文件原盘号，包括不连续的 1、7 等编号，不重写 G-code 或切片工艺。
- 同一批任务共用不可变原文件；删除其中一项不影响其他项，最后一个引用删除后才清理文件。备份会保留各任务引用。
- 没有缩略图时显示占位提示。只读取受限大小 PNG，重新编码为最多 160×160；不加载 SVG、模型几何或可执行内容。检查报告也可能包含这些预览，分享前核对文件名与参数。
- 预估用量缺失时必须填写；已知用量不能调低到切片估算以下。材质必须匹配，颜色选择不把 PLA 工艺变成 PETG 工艺。

### 上传中断怎么办

批量上传前浏览器保存本账号的导入编号及原安排。结果不明确时先点“核对结果”；已保存的批次会返回原任务编号。仍未查到时，可用**同一编号**重试原文件。系统不会自行重复上传或启动打印。

不要为了重试换浏览器、清除站点数据或重新创建一份新安排；这样可能丢失本地恢复编号。若已经清除，请先在“我的任务”核对后再操作。被服务器明确拒绝的草稿允许修改，但继续沿用原编号。若旧请求先完成，系统返回原批次，需到任务列表核对最终安排。

### 部署与回退

该升级同时包含前端和 NAS 后端，不能只替换云端网页。云端负责静态文件与访问转发；NAS 负责解析、批次事务、库存校验和盘号保存。应在没有打印、传输及待确认指令的维护窗口统一升级，先备份工作空间及主密钥，保存原版本程序和网页。

工作空间数据库升级至 **schema 5**，可从旧版本迁移。旧程序不认识新选盘语义，不能直接回退打开新数据库；回退须同时恢复匹配的升级前工作空间备份，会回退升级后产生的数据。不要在有新任务或新结算后直接覆盖回旧快照。

A1 无 AMS 的现场检查见 [A1 接入准备](A1_SETUP.md)。软件测试通过不代表 A1 已完成实机验收。

## English

Export a **sliced 3MF** for the actual printer/nozzle/plate/material from Bambu Studio. After inspection, choose plates by their original IDs, previews and material/color estimates. A single-plate file is selected automatically; a multi-plate file starts with no selection. Clicking Next without a selection opens a plate chooser. One selected plate uses the normal job form, showing its color and exact sliced consumption and filling in its own estimated time and rounded-up reservation. Several selected plates open a per-plate batch review, up to 16 plates.

Choose a compatible printer or assign later. Review each plate’s spool, reserved grams and estimated minutes. Color cards display spool identity, material, available stock and registered mount. Saving creates independent jobs in one transaction: any invalid arrangement rolls back the entire batch. Combined reservations for a shared spool cannot exceed available stock. Actual inventory is deducted only during consumption settlement.

Batch intake currently accepts single-nozzle, single-filament plates. Import multi-filament plates individually and use the supported preparation workflow. Existing restrictions on multi-nozzle starts remain. Plate IDs are preserved, including sparse IDs such as 1 and 7. Each selected plate requires saved feed preparation before transfer/start; changing it in preparation updates the stored plate and known time estimate.

**Queueing does not start printing or authorize unattended consecutive prints.** Clear the plate and explicitly confirm each real start. The original archive/G-code remains unchanged. Shared source files are retained until the final referencing job is deleted. Optional PNG previews are bounded, decoded and re-encoded at up to 160×160; geometry/SVG are not rendered.

In Printer queues, expand **Change order** to move a waiting job up/down or **Make next**. Making a job next requires confirmation and only changes its waiting position; it does not transfer or start it. The next job is labeled, and other jobs cannot bypass the head. Administrators can reorder the printer's waiting jobs. Members can reorder their own consecutive waiting jobs without overtaking another member. Transferred, executing and review-pending jobs stay in place; spool reservations and preparation stay unchanged. A concurrent queue change rejects the stale request. After an uncertain response, explicitly **Reread queue** and review the result; no reorder is retried automatically.

An uncertain upload retains the original request ID and arrangement in account-scoped browser session storage. Check the receipt first; a retry with that same ID returns the original jobs instead of creating duplicates. There are no automatic upload/start retries. Avoid clearing browser data or switching browsers to retry. If the recovery ID is lost, inspect My jobs before creating another import. Corrections after a confirmed rejection retain the same ID; if an earlier request already won, review its original jobs.

Upgrade the NAS application and cloud static files together during an idle maintenance window, after backing up the workspace, master key, binaries and web files. The NAS owns inspection, transactions and reservations; the cloud serves the UI and proxies requests. This feature advances workspace schema to **5**. A pre-feature binary cannot safely read its plate semantics; rollback requires the matching pre-upgrade workspace backup and loses intervening changes. Do not overwrite newer jobs/settlements with an old snapshot.

See [A1 without AMS setup](A1_SETUP.md). A1 physical validation remains pending.
