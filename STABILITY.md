# A1 与运行稳定性更新 / A1 and runtime stability

本批包含手动移动面板与稳定性改进，保持 Beta 团队管理定位，不增加切片工作台。

- 控制只发送一次非保留 QoS 0 消息，避免连接恢复时重发旧控制。不把发送回执当成执行完成；明确拒绝优先，只有发送后的匹配遥测才能确认执行。没有证据仍进入人工核对。
- A1 照明灯已通过现场亮灭验证；固定遥测不作为真实亮灭证据，界面显示上次请求。A1 mini 灯光仍未开放。温度可显示两位小数，未知值仍显示未知；其他整数协议字段没有放宽。
- 相机模式按设备实际帧率说明，A1 显示原厂低帧率提示；传输模式不会把原厂低帧率相机变成 8 fps。
- 有 pending/uncertain 指令或执行错误的任务不能删除。新建普通或多盘任务时跳过历史命令及导入回执中的任务编号，防止旧身份被重新利用。
- 管理员在设备控制台的操作记录中可核对“孤立启动记录”。需现场检查并输入准确的 job-N，服务端再次匹配新鲜设备身份。它只追加审计并解除该条锁，不重建任务、文件、归属或库存，不扣耗材、不发控制、不伪造完成邮件。
- 手动移动首版仍限 X2D 已验证的归位及单步模式；A1 移动保持禁用。

## 升级期间继续打印

新版 UpdateKit 源码增加 `nas --allow-running` / `cloud --allow-running`，仍需 `--apply` 才执行。默认保持要求空闲。显式选项只允许在线、新鲜、无告警且可与已登记任务准确匹配的稳定 RUNNING；传输、启动、暂停、待核对指令、恢复和未认领任务仍拦截。数据备份及停服前后核对继续执行。更新不向打印机发送暂停、停止或重新启动，但监控与远程操作会短暂中断。

本次只更新运行部署，不表示已发布新的 GitHub Release 或覆盖旧下载资产。

## English

This batch includes the manual motion panel and keeps Beta focused on team printer management. It adds no model workbench.

Controls are sent once using non-retained QoS 0. Fresh, matching device observations—not transport receipts—confirm execution; explicit rejection wins, and uncertainty still requires review. A1 light switching was confirmed onsite; acceptance receipts and static light telemetry are not claimed as physical-state measurements. A1 mini lights remain unvalidated. Valid decimal temperatures display to two decimals without relaxing integer protocol checks. Camera labels explain source-limited frame rate rather than promising 8 fps on A1 hardware.

Jobs with pending/uncertain commands or execution errors cannot be deleted. Single and multi-plate imports avoid reusing IDs present in command history or import receipts. Administrators can review orphaned start records from the printer console history after checking onsite and typing the exact job-N. The server rechecks fresh identity. Review appends an audit and releases only that command lock; it never reconstructs ownership, stock, files or completion notifications, and sends no printer commands.

Updated UpdateKit source supports explicit `nas --allow-running` and `cloud --allow-running`, still requiring `--apply`. Default behavior remains idle-only. Stable running prints must be fresh, alert-free and matched to registered jobs. Transfers, starts, pauses, uncertain commands, restores and unknown jobs remain blocked. Backups and before/after checks remain mandatory. Printer execution is independent, while monitoring and remote access briefly disconnect during service restart.

## A1 light and missing completion records (2026-10-28)

The A1 chamber_light command was verified onsite in both directions. Its firmware continued to report `on` after the light physically turned off. LayerHub therefore exposes one light switch with an unknown initial state and a clearly labeled last request. A matching positive `ledctrl` receipt records `accepted`, not measured hardware success. Explicit errors take priority, silence remains uncertain, and physical commands are never automatically retried. A1 mini lights and A1 auxiliary/chamber fans remain unavailable until independently supported; nozzle, bed, part fan and printing speed retain their existing guards. A1 axis movement remains unvalidated.

A matching `FINISH` / 100% report with an audited start but no original job now keeps the printer awaiting collection and usage. The observation is persisted without changing jobs or stock, so clearing device telemetry or restarting the service does not release that hold. This also blocks the next transfer/start. Reviewing an old start receipt alone never settles material.

Administrators can reconcile a missing completed job by entering its exact task ID, confirming a cleared bed, selecting the actual spools and entering usage plus a note. This is an explicit, atomic correction recorded separately from normal job settlement. It does not recreate a filename, owner or estimate, and sends no guessed completion mail. Values are retained to three decimal places; each spool is booked upward to whole grams consistently with normal settlement. Private spool ownership and other reservations are respected. Duplicate submissions cannot deduct inventory twice. Unclear or changed device identity, stale telemetry, alarms and outstanding controls prevent reconciliation.


## 中文：完成记录补登记

有启动审计且身份匹配的任务在 FINISH / 100% 后缺失原记录时，设备保持待取件、待用量登记，并持久保留核对状态；不自动扣库存或放行下一任务。管理员需确认任务标识、取件清板、实际料卷、用量和说明，原子补登记只执行一次，不伪造原任务归属或完成邮件。公共料卷与个人共享是不同字段，补登记沿用耗材库编辑权限。

A1 只有灯光亮灭及云端接受回执完成对应现场验证，移动和完整打印收尾闭环仍未验收；不能将此推广到所有固件或机型。界面字体、主内容区与侧栏滚动、许可证页面及 Studio 快捷入口不涉及自动打印或库存结算。
