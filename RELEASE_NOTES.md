# LayerHub v0.1.2-beta · Print preparation, maintenance and Studio

**Recommended: LayerHub Beta + Bambu Studio.** This release packages the updates developed since v0.1.1-beta; earlier releases and Alpha remain intact.

- Combine sliced-3MF inspection, plate selection, device/spool confirmation and print options in one preparation view. For one plate on a ready printer, explicit clearance allows one transfer/start request; multiple plates only queue. Cancel never-started tasks without deleting history or deducting material.
- Explain the number of separate print runs and retain each plate's estimates, colors and identity. Reorder a printer's queue and explicitly choose the next job; never start later plates automatically.
- Add X2D manual homing and individual XYZ small steps with fresh-state checks, onsite clearance and audited, non-repeating commands. [Motion scope](MOTION.md).
- Improve A1 decimal readings, source-limited camera hints and light switching. Onsite on/off observations validate the light; static telemetry is not physical-state proof. A1 XYZ movement and the complete print/collection/material acceptance cycle remain unverified.
- Keep missing completed-task records awaiting explicit collection and usage reconciliation. Public spools are correctly distinguished from personally shared stock. No automatic material deduction or guessed completion mail.
- Repair font application, long-page/sidebar scrolling, and source/license presentation. Keep English and Chinese support, with deterministic locale generation across Windows/Linux line endings.
- Put a local Bambu Studio shortcut first in the sidebar. Users enter a Windows EXE path and import a generated per-user `.reg` file once. No resident helper or remote launch service is installed; paths stay in the browser. [Setup/removal](STUDIO_SHORTCUT.md).
- Include the updated Windows/Linux Download / Update Kit. Default upgrades require idle printers; explicit `--allow-running` still requires a fresh, alert-free known job and never sends pause/stop commands. Monitoring briefly disconnects during a backend restart.

## Downloads and upgrade

Download a platform application, optionally the standalone Update Kit, and **SHA256SUMS**:

- `LayerHub-0.1.2-beta-windows-amd64.zip`
- `LayerHub-0.1.2-beta-linux-amd64.tar.gz`
- `LayerHub-0.1.2-beta-source.tar.gz`
- `LayerHub-Download-UpdateKit-0.1.2-beta.zip`

Verify checksums and extract into a new directory. Back up the full workspace and master key. Coordinate NAS backend and cloud UI upgrades. Schema remains **5** with additive reconciliation records; do not assume an older program preserves these new holds. Rollback requires review and the matching pre-upgrade backup, accounting for any newer jobs and inventory changes.

Linux updates target existing native Debian/Ubuntu amd64 systemd installations with Python 3. ARM, Docker, first-time two-host network/certificate setup, universal printer/firmware compatibility and unattended physical acceptance are not claimed. Publishing this release does not restart or upgrade production services.

[Deployment](DEPLOYMENT.md) · [Print preparation](PRINT_PREPARATION.md) · [A1](A1_SETUP.md) · [Studio shortcut](STUDIO_SHORTCUT.md) · [Validation](validation/README.md)

## 中文

**推荐 LayerHub Beta 配合 Bambu Studio。** v0.1.2-beta 将上一版之后的改动统一打包，保留旧版和 Alpha 标签与附件。

- 整合切片文件检查、选盘、设备/料卷确认和打印选项。单盘且设备就绪时，明确确认清板后只提交一次传输与启动；多盘只入队，不自动连续打印。
- 显示独立打印次数、逐盘用量与颜色；可调整本机队列顺序、指定下一项。尚未启动的任务可以取消，保留历史并释放预留，不扣库存。
- X2D 增加原生归位与 XYZ 单次小步移动，保留现场清板、新鲜状态和结果核对；不重发未知结果的动作。
- A1 增加小数温度、低帧率相机说明和经现场亮灭验证的照明灯。固定遥测不当成真实灯光状态；A1 移动及完整打印收尾闭环仍未验收。
- 原任务缺失的已完成打印保持待取件、待用量登记；补登记正确识别公共耗材，只有明确确认后才扣减一次，不伪造归属或完成邮件。
- 修复个性化字体、长页面与侧栏滚动，优化源码与许可证页面；修复 Windows/Linux 换行差异引发的语言文件检查失败。
- 左侧第一项可快捷打开本机 Bambu Studio。填写 Windows 主程序路径，首次导入生成的当前用户 `.reg` 配置；路径只存浏览器，无需常驻客户端。
- 同步打包更新工具。默认要求空闲；显式允许打印中升级仍需新鲜、无告警且已认领的任务。升级不向打印机发送暂停或停止，后台重启期间监控会短暂断开。

下载对应应用包、可选的下载/更新工具包以及 **SHA256SUMS**，校验后解压到新目录。升级前备份完整工作空间和主密钥，协调更新 NAS 与云端。数据库仍为 **schema 5**，新增补登记记录；旧程序不能保证识别新核对状态，回退需审核并使用匹配备份，不能覆盖后续任务和库存变化。

127 项前端/语言/配置生成测试、Go 测试与 vet、跨平台构建、临时离线工作空间检查及附件校验用于本次交付，不自动打印或发送测试邮件。独立部署仍须按指南验收设备、网络和固件。GitHub 发布不等于生产服务已升级。
