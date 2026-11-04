# LayerHub v0.1.3-beta · Background uploads, materials and personalization

**Use Bambu Studio for slicing, then LayerHub Beta for shared printers.** This release bundles the source updates after v0.1.2-beta into Windows/Linux applications, matching source and the Download / Update Kit. Older releases and the Alpha channel remain available.

## What changed

- **Upload → confirmation → start or queue:** durable NAS-side background parsing, recovery from the background file list, automatic confirmation when parsing finishes, clearer cancellation and submission-result review. The upload itself must finish before the page is closed. Multi-plate files keep per-plate time, weight, color and queue identity; only the explicitly selected first plate can start.
- **Materials:** preserve TPU-AMS and unlisted material values in the editor instead of falling back to PLA; add protected deletion of unused/unmounted records and visible printer/library mismatches. **Update from printer** corrects the existing mounted record's material and color, preserving grams, ownership and reservations. It does **not** treat a changed report as a new physical roll or unmount the old one automatically.
- **Printer status and controls:** edit a printer IP while retaining its identity/credentials; display AMS drying stages and prevent starting while drying; retain PrintError/HMS observation details for future incidents. A1 temperatures are rounded for display; control values are unchanged. X2D AMS PETG load/change/unload and motion-status refresh are included within their documented validation scope.
- **Personalization:** system-following/light/dark modes, accent presets/custom colors, density, more local font choices and account-scoped browser preferences. Actual filament colors and semantic status colors retain their meanings.
- **Bambu Studio shortcut:** a fixed single-instance launch option helps reuse an existing Studio process. Paths remain in the member's browser and require that member's Windows protocol registration.
- **Illustrated guides:** English and Chinese Studio → sliced 3MF → upload → start/queue instructions, with all six supplied screenshots, are included in the repository and applications.

## Validation and limitations

Local frontend/language/shortcut checks, Go tests/vet, Windows/Linux builds, disposable-workspace smoke checks and package-content/checksum verification are recorded for this release. GitHub CI is checked before publication. These checks do not replace attended hardware acceptance.

**Experimental X2D external spool / soft TPU:** the file's physical nozzle selects the exact external source; fresh loaded-source, profile and nozzle checks are required. It is implemented and tested offline/with fixture APIs, but no real external-spool print was started for acceptance. Do not describe this as fully hardware-validated. Printer presets must match the actual roll and slice; TPU for AMS and TPU 90A are distinct presets.

Physical roll replacement remains a manual inventory/mount operation. This release does not add automatic old-roll unmount/new-roll assignment. Camera frame rate remains limited by each printer. Background parsing persists completed uploads, not interrupted upload bytes. Existing firmware/authorization checks and uncertain-command holds remain in effect.

## Downloads and upgrade

- `LayerHub-0.1.3-beta-windows-amd64.zip`
- `LayerHub-0.1.3-beta-linux-amd64.tar.gz`
- `LayerHub-0.1.3-beta-source.tar.gz`
- `LayerHub-Download-UpdateKit-0.1.3-beta.zip`
- `SHA256SUMS`

Verify checksums, extract to a new directory and back up the entire existing workspace including its master key. Coordinate NAS backend and cloud UI updates; prefer idle printers. Schema remains **5** with additive records. Keep the matching backup and old release for reviewed recovery rather than overwriting newer jobs or stock. The application defaults to offline device mode and remote print start disabled. Publishing this release does not restart, upgrade or control production devices.

[Illustrated guide](USAGE.md) · [Deployment](DEPLOYMENT.md) · [Update Kit](deploy/update-kit/README.md) · [Changes](CHANGELOG.md)

## 中文说明

**v0.1.3-beta 将 v0.1.2-beta 之后的更新统一打包。推荐 Bambu Studio 切片，LayerHub Beta 管理共享打印机。**

1. **上传与排队更顺畅：** 文件上传完毕后由NAS后台持久化解析，解析完成进入确认页；可返回后台文件列表继续。确认设备、打印盘与实际料卷后，按条件立即启动首盘或仅入队；其余盘仍需清板后逐次确认。上传未完成就关闭页面仍可能中断。
2. **耗材管理：** 修复TPU-AMS/未列出材质误显示PLA；加入耗材删除保护、挂载信息差异提示及“按打印机更新”。更新只校正当前记录的材质和颜色，保留克数、归属与预留；尚未实现换卷后自动卸载旧卷、挂载新卷。
3. **设备功能：** 可修改打印机IP；识别AMS烘干状态并限制此时启动；保留后续PrintError/HMS告警信息；A1温度改为整数显示。纳入已记录范围内的X2D AMS PETG进换退料、移动状态刷新与相关控制修复。
4. **个性化：** 跟随系统/浅色/深色、预设及自定义主题色、界面密度和更多本机字体。偏好按账户存于浏览器，实际耗材颜色保留原义。
5. **Studio和文档：** 单实例打开Studio；新应用包附带六张截图的中英文使用流程，说明切片导出、上传、选料、立即打印与队列，以及完工后的取件和用量登记。

**验证边界：** X2D外置料架/软TPU启动路径目前为实验性功能，只完成离线与模拟接口验证，尚未完成真实打印验收；机器预设仍需与实物、切片一致。此次发布不会操作打印机或自动升级生产环境。摄像头帧率受机型限制，后台解析不包含断点续传上传。

下载并校验SHA256后解压到新目录；升级前备份完整工作空间及主密钥，协调更新NAS与云端，优先选择设备空闲时。数据库仍为schema 5，并包含新增记录；回退前保留匹配备份，避免覆盖新任务与库存。新装默认离线，远程启动默认关闭。

[中文图文流程](USAGE.zh-CN.md) · [部署指导](01-部署指导-云服务器-NAS-打印机.md) · [更新工具](deploy/update-kit/README.zh-CN.md)
