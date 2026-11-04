# Unreleased / 未发布

## 2026-11-04 source update / 源码更新

- Add English and Simplified Chinese illustrated usage guides with six user-provided screenshots; expand the README workflow from Studio export to immediate start or per-printer queueing.
- 新增六张截图的中英文图文使用指南，明确已切片3MF格式、后台解析、实际料卷选择、立即开始与仅入队、多盘逐次打印和完工结算。
- Add durable background inspections with automatic handoff to print confirmation; cancellation and ambiguous-result recovery retain explicit start semantics. Add printer IP editing and Studio single-instance launching.
- 新增持久化后台解析、解析后进入确认、上传取消与结果核对；增加设备IP修改与Studio单实例打开。
- Add AMS drying visibility/start guards, PrintError/HMS observation details, protected spool deletion, integer A1 temperature display and broader appearance preferences.
- 补充AMS烘干展示与启动检查、PrintError/HMS告警详情、耗材删除保护、A1整数温度及主题字体设置。
- Fix material selectors falling back to PLA for TPU-AMS or unlisted materials. Add explicit printer-report material/color sync preserving grams and ownership. This corrects an existing record; replacement-roll unmount/remount remains pending.
- 修复TPU-AMS等未列出材质误选PLA，增加按机器更新材质颜色并保留克数与归属；换卷后的原卷卸载与新卷挂载仍待实现。
- Add X2D external-spool routing with exact physical nozzle/source and fresh profile checks. Offline and fixture-browser tests passed; live print acceptance and production deployment of this route remain pending.
- 新增X2D外置单料卷路径和物理喷嘴/料位、新鲜预设检查；离线与模拟浏览器验证通过，实机打印验收和这部分生产切换仍待完成。
- These are main-branch source changes, not replacements for existing version tags or download archives. 本次仅更新main源码，不重写旧版本标签或安装包。

## Earlier development notes / 先前开发记录

The notes below retain their original dated validation/deployment scope. They do not describe all later hardware checks. 以下保留原日期的验证和部署范围，不代表后续所有实测状态。

- Add guarded X2D + AMS 2 Pro PETG load/change/unload controls with actual colour slots, confirmation, one-shot sending and sensor-based completion. Add read-only motion refresh and explicit missing-axis/clearance reasons; align motion freshness to the full-snapshot cadence.
- 新增 X2D + AMS 2 Pro 的 PETG 进换退料界面，实际颜色选槽、独立确认、单次发送与传感器完成检查；新增只读移动状态刷新和明确的回零/现场确认提示，调整状态有效期以匹配完整快照周期。

- Add ten local font choices from the requested reference, including STZhongsong, STSong, STXihei, STKaiti, YouYuan, LiSu, STHupo, Bahnschrift, a themed combination and monospace; keep old font preferences and defaults. Use installed system fonts with fallbacks rather than bundling font files.
- 按参考图新增十个本机字体选项，总计 20 种；保留原字体偏好和默认值，未安装字体时使用后备字形，不打包本机字体文件。

- Add six accent presets and validated custom colours, light/dark/system modes, independent layout density, whole-workspace previews with reversible cancel/reset, and account-scoped browser storage. Preserve existing font preferences, semantic device states and actual filament colours. The appearance UI was deployed to the existing cloud and NAS on 2026-10-29 without a service restart; no new GitHub release has been published. See [appearance settings](APPEARANCE.md).
- 新增六种主题色与自定义颜色、浅色/深色/跟随系统、独立页面密度及全工作区实时预览；取消/重置可还原，偏好按账户保存在浏览器。保留旧字体设置及设备状态和实际耗材颜色。个性化界面已于 2026-10-29 无重启上线至现有云端与 NAS，尚未发布新的 GitHub 版本，见[个性化说明](APPEARANCE.md)。

- Use ten-percent fan controls (0, 10, …, 100) with 20/50/80 presets. Reject off-level input before creating a command; do not silently round or resend. Fan-off confirmation requires a fresh zero report; a zero report cannot confirm fan start.
- 风扇按整十档调整，快捷值改为 20/50/80%；非整档输入在创建指令前拒绝，不静默改值或重发。关闭必须收到新鲜的零值回报，零值也不能确认风扇已启动。
- Record the October 29 control checks and the remaining A1 motion work in [CONTROL_VALIDATION.md](CONTROL_VALIDATION.md). These changes are local and have not been deployed or included in a new release.
- 整理 10 月 29 日控制实测及 A1 移动适配边界，见[控制验证记录](CONTROL_VALIDATION.md)。本项为本地待发布修改，尚未部署或发布新版本。

# 0.1.2-beta — Print preparation, maintenance and Studio / 打印准备、维护与 Studio

- A combined upload-to-print preparation view, per-plate queue order, explicit next-job selection, and cancellation of never-started tasks. Later plates never start automatically.
- X2D manual homing and individual XYZ small steps with onsite clearance, freshness checks and command review. [Motion scope and physical evidence](MOTION.md).
- A1 decimal readings, source-limited camera hints and an onsite-verified light switch. A light receipt means accepted; the UI shows the last request rather than trusting static light telemetry. A1 motion and a complete print/collection/material acceptance cycle remain unverified.
- Missing completed-task records remain awaiting explicit collection/usage reconciliation; they do not silently release the machine or deduct stock. [Runtime stability](STABILITY.md).
- Working font preferences, scrollable long pages and sidebar, and a redesigned source/license page.
- A first-position **Bambu Studio** shortcut with a browser-local Windows EXE path and a one-time, user-imported registration file. [Setup and removal](STUDIO_SHORTCUT.md).

- 合并上传到打印前的准备步骤，逐盘排队、调整顺序、指定下一项，以及取消尚未启动的任务；不自动连续开打。
- X2D 原生归位与 XYZ 小步移动，保留现场清板、状态新鲜度和结果核对要求，见[移动范围与实机证据](MOTION.md)。
- A1 小数温度、低帧率相机说明，以及现场验证过亮灭的照明灯。灯光回执仅表示接受，界面显示上次请求。A1 移动与完整打印、取件和耗材结算闭环仍未验收通过。
- 完成后缺失原任务的设备保持待登记状态；必须明确确认取件及实际用量，不自动放行或扣减库存，见[稳定性说明](STABILITY.md)。
- 修复字体应用和长页面、侧栏滚动，重新整理源码与许可证页面。
- 左侧导航第一项新增 **Bambu Studio**，本机路径只存浏览器，首次由用户导入 Windows 配置，见[设置与移除方法](STUDIO_SHORTCUT.md)。

- Make generated locale files stable across LF/CRLF checkouts; add a clean temporary-project regression.
- 修复 Windows/Linux 换行差异导致语言文件校验失败的问题。

# 0.1.1-beta — Reliable controls, updates and multi-plate intake / 控制、升级与多盘导入

- Improve control response feedback and preserve console focus, tabs and camera during telemetry updates. Reconcile printer-side stops without treating them as successful completions; release readiness only after manual collection/settlement.
- Ship a standalone Windows/Linux download/update kit with channel selection, verification, offline transfer, idle checks, backups and coordinated NAS/cloud updates.
- Preserve the explicitly selected sliced plate through queueing and print preparation; never silently choose the first plate of a multi-plate file.
- Offer bounded PNG thumbnails and plate-specific time, material/color and reservation estimates.
- Enqueue 1–16 single-filament plates atomically, each as a separate job. Owner-scoped import receipts and stable request IDs prevent duplicate batches after an uncertain response. No automatic print starts.
- Preserve shared source files until the final referencing job is deleted; validate combined stock reservations, material and printer compatibility.
- Disable AMS preparation for bare printers; add A1 external-spool regression tests and bilingual on-site setup guidance. Physical A1 validation is pending.
- Workspace schema advances from 4 to 5. Back up before upgrading. Reverting to a pre-feature binary requires restoring the matching pre-upgrade workspace; do not point it at the migrated database.
- 本版合并：明确选盘、逐盘颜色与预览、整批原子入队、断网重试防重复、共用文件保护、中英文界面及 A1 无 AMS 准备。与下载/更新工具一并发布；真实 A1 接入待现场验证。

# 0.1.0-beta — Team printer management / 团队打印机管理

Start the Beta mainline. Remove the model/slicing workbench, its geometry/editor APIs, shared model snapshots, Three.js and NAS engine runtime. Accept only actual sliced 3MF. Inspection now finishes inline on the dashboard without a purpose chooser. Keep accounts, device control, queues, physical inventory, notifications, diagnostics and backups; retain legacy project rows without exposing editor APIs. Add bilingual migration/deployment guidance and beta packages. Alpha tags/releases remain available.

Beta 成为持续维护主线：移除模型工作台及在线切片，只接受已切片 3MF，保留完整团队打印管理。历史工程数据保留备份，旧模型接口返回 410，不再依赖 NAS 切片引擎。

# 0.4.0-alpha — File intake and print preparation / 文件导入与打印准备

- Inspect 3MF contents from either file entry; independently identify editable geometry and valid sliced plates instead of trusting the filename.
- Offer explicit existing-slice and editable-geometry routes; unsupported geometry does not hide a valid sliced route. Neither choice starts a printer from the inspector.
- Show per-plate material/color codes, time/weight estimates and original parameter references; compare reference values with workbench defaults before geometry import.
- Prefill rounded single-plate queue estimates; leave missing and multi-plate estimates blank. No automatic spool selection or actual stock deduction.
- Export personal inspection reports locally; require authenticated CSRF-protected inspection, bounded temporary uploads and cleanup. Cancelled/late reads cannot replace drafts or other pages.
- Reject unsliced/invalid 3MF queue uploads on the backend and reject archive links. Existing import digest/receipt checks and print-start guards remain.
- Ship complete Chinese/English UI, import guidance and updated deployment packages in one bundled release.
- 本次合并内容识别、双入口、原参数对照、逐盘颜色与预计值、报告导出及后端队列校验，一并更新中英文文档；不新增机型、多色或双喷嘴启动能力。

# 0.3.0-alpha — Deployment diagnostics / 部署与连接诊断

- Add local `--doctor` inspection before startup, bilingual/JSON output and opt-in temporary-file write probes. No workspace initialization, device connections, slicing or SMTP tests.
- Add administrator-only System diagnostics with separate browser, independent gateway, NAS application and printer evidence; distinguish unknowns, optional components, stale telemetry and control authorization restrictions.
- Add redacted report export, explicit freshness expiry and actionable bilingual remedies. Results are cleared on permission loss; checks never retry commands.
- Add a standalone Nginx `/gateway-healthz` marker and bilingual diagnostic guidance.
- 新增部署自检、管理员连接诊断、脱敏导出与独立网关标记；不会启动打印或发送测试邮件。

# 0.2.0-alpha

- Simplified Chinese / English interface across sign-in, dashboard, materials, printer controls, queues, security and slicing.
- Persistent account/browser language; locale-aware dates/numbers, server errors and future verification/job/material/device emails. NAS outage notices are bilingual.
- Protected filenames/member names and unchanged protocol IDs, spool mappings and numerical settings.
- Draft acknowledgment before language reload; failed/uncertain saves preserve the current page.
- Locally shipped catalogs, placeholder/coverage checks and English workflow tests. No online translation service.

新增完整中英文界面与账户语言偏好，覆盖登录、看板、设备、耗材、队列、切片和安全设置。日期、系统提示及后续邮件随账户语言显示，NAS 掉线告警为双语。用户内容及协议参数保持；语言切换前等待草稿保存，失败时保留页面。新增语言包检查及英文流程测试。

# 0.1.0-alpha

Initial independent-deployment alpha: three-tier roles and sessions, verified email notifications, per-printer queues and spool inventory; private STL/3MF geometry projects, real slicing and layer paths; versioned team snapshots; NAS application + restricted cloud HTTPS gateway templates; Windows/Linux amd64 packages, corresponding source, licenses and bilingual deployment/acceptance documentation.

Hardware evidence covers one tested X2D firmware/module configuration. Catalog artwork does not establish compatibility. Multicolor/multi-nozzle start, motion/filament controls, per-object support, mesh cutting and a desktop installer are outside this release's claims.

## 简体中文

首次外部部署测试交付，基于已经验证的 Go/SQLite 版本整理。

- 三级账号权限、个人会话管理、邮箱验证与通知、单机队列及逐卷耗材。
- 私有 STL/3MF 几何工程、对象工艺、多盘、真实切片与逐层路径。
- 团队工程快照、历史版本、独立复制和撤回。
- 独立 NAS 运行、云端 HTTPS 网关和主动 SSH 反向隧道部署模板。
- Windows/Linux amd64 二进制、对应源码、依赖许可证、部署指南和验收清单。

当前实机验证集中于一台 X2D 的特定固件/模块组合，不能据此推断机型图鉴中全部设备兼容。多色/双喷嘴启动、运动/进退料、对象支撑、模型切割及客户端安装器不作为本版能力承诺。
