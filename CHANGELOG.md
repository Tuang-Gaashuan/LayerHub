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
