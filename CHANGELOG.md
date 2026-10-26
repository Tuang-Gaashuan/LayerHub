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
