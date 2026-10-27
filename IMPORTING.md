# Bringing files into LayerHub / 文件导入

0.4.0-alpha · [English README](README.md) · [中文主页](README.zh-CN.md)

## English

Choose a 3MF from **Model workbench → Import models** or **Team dashboard → Upload sliced file**. The same inspector checks the archive contents. The filename does not decide whether the file is editable or sliced. Inspection requires sign-in and the current session's CSRF token; its temporary upload is removed after the request. It creates no saved model, project, job, spool reservation, printer command or email.

| Choice | What happens | What stays explicit |
|---|---|---|
| **Use existing slice** | Return to the dashboard with the original file selected. Paths and embedded process are not regenerated. Preview each plate's material, color code, estimated time and weight. | Confirm the device and visible spool before adding to the queue. Review plate, nozzle and feed mapping in print preparation. Nothing starts from the inspection dialog. |
| **Import editable geometry** | Review geometry and original parameter references, then explicitly agree to import a new private project. Existing receipt-based retries remain idempotent. | Geometry is repacked and grounded. Original X/Y coordinates and Studio process settings are not inherited. The active draft is not silently replaced. |

A file may offer both routes, one route, or neither. An unsupported modifier/painting feature in geometry does not disable an otherwise valid sliced route. A usable slice is **not** proof that the printer can start it: existing model, diameter, nozzle, plate, material, authorization and fresh-device checks still apply.

### Original settings and estimates

The inspector reads only a small display allowlist: printer model, nozzle diameters, plate, material types, color strings, layer height, infill density and pattern. Missing values stay unknown. Embedded macros, custom G-code and arbitrary configuration are not exposed as reference fields or applied to the slicing engine. On the geometry confirmation screen, file values appear beside the workbench defaults: X2D, 0.4 mm, textured PEI, single-color PETG Basic, 0.20 mm layer height and 15% grid infill. Review every plate and re-slice.

For **one sliced plate**, known time is rounded up to whole minutes and total material usage to whole grams for the initial queue reservation. All estimates remain editable; missing or out-of-range values stay blank. For **several sliced plates**, estimates are left blank instead of guessing the first plate. Enter values deliberately, then select the intended plate and verify its consumption during print preparation. No spool or AMS mapping is automatically chosen; estimates do not deduct actual stock.

### Limits and recovery

- Inspection accepts one `.3mf` at a time, up to 500 MB or the server's smaller configured upload limit. The bounded geometry importer still requires at most 32 MB and retains its existing object/triangle/plate limits.
- To keep Studio process settings, finish slicing in Studio and export a sliced plate file. Saving a model project alone does not produce printable paths.
- To edit unsupported geometry, export solid STL from the original slicer or save an ordinary mesh 3MF without painting, modifiers or special volumes. Geometry-only files belong in Model workbench, not the direct print queue.
- Cancellation or timeout leaves projects, drafts and jobs untouched. Late results cannot reopen a dismissed inspection or replace another page. Retrying inspection is read-only; committing an import still requires explicit consent and a matching file digest.
- The backend rejects unsliced/invalid 3MF uploads before creating a queue job or reserving inventory, including API clients that bypass the chooser. Raw G-code retains its existing separate workflow and is not classified by this 3MF inspector.
- **Export inspection report** downloads JSON locally. It contains this filename and the allowlisted references/plate/model summaries. It is a personal file report, not the administrator's redacted deployment report; review it before sharing.

No new engine, firmware compatibility, multicolor start or dual-nozzle printing support is claimed by this release.

## 简体中文

从**模型工作台 → 导入模型**或**协作看板 → 上传切片文件**选择 3MF，都会先进入同一检查界面。系统读取实际内容，不依靠文件名是否含 `gcode` 判断。检查需要登录与会话校验；临时上传文件会在请求结束后清理，不创建模型、工程、任务、耗材预留、设备指令或邮件。

| 选择 | 处理方式 | 仍需确认 |
|---|---|---|
| **使用已有切片** | 回到看板并选中原文件，保留原切片路径与工艺；提前查看各盘材质、颜色代码、预计时间与重量。 | 确认设备和可见料卷后才加入队列，打印准备时核对具体盘、喷嘴和供料映射；检查页面不会启动打印。 |
| **导入可编辑模型** | 查看几何和原参数参考，明确同意后导入为新的私有工程；结果不明时沿用同一导入标识核对与重试。 | 重新排版并贴底，不保留原 X/Y 坐标，也不继承 Studio 工艺；不会悄悄替换现有草稿。 |

一个文件可能支持两个入口、其中一个或均不支持。模型包含暂不支持的修改器或涂色，不会连带禁用有效的切片入口。检测到切片也不代表能在目标设备启动，机型、喷嘴、打印板、材质、授权与新鲜状态等校验仍保留。

### 原参数与预计值

仅提取少量供显示的字段：机型、喷嘴直径、打印板、材质、颜色、层高、填充密度和图案。缺失项保持未知；机器宏、自定义 G-code 和任意配置不会进入参考字段或传给切片引擎。几何导入确认页并列显示原参数与工作台默认值：X2D、0.4 mm、纹理 PEI 板、单色 PETG Basic、0.20 mm 层高、15% 网格填充。导入后需要逐盘核对并重新切片。

**单盘切片**的已知用时、总用量分别向上取整到分钟和克，预填初始排队预留；允许修改，缺失或超限的项留空。**多盘文件**不会猜选第一盘，预计值留空，需要手动填写，之后在打印准备中选择实际打印盘并核对用量。系统不自动选择料卷或 AMS 映射，也不提前扣减实际库存。

### 限制与处理办法

- 一次检查一个 3MF，最大 500 MB，服务器配置更小限制时以服务器为准；几何导入仍为 32 MB，并保留对象数、三角面数和打印盘限制。
- 希望保留 Studio 工艺：先在 Studio 完成切片，再导出切片盘文件；只保存模型工程不产生可直接打印的路径。
- 希望编辑不受支持的工程：在原切片器导出实体 STL，或另存不含涂色、修改器与特殊体积的普通网格 3MF。纯模型工程应进入工作台，不能直接加入打印队列。
- 取消或超时不改变草稿和任务；迟到的检查结果不能重新弹窗或覆盖其他页面。检查可重新执行；真正导入仍要求明确确认与文件摘要一致。
- 后端会在建任务、预留库存前拒绝未切片或无效的 3MF，也适用于绕过界面的 API 请求。原始 G-code 沿用原流程，不由本检查器分类。
- **导出检查报告**在本机下载 JSON，包含文件名及白名单参数、打印盘和模型摘要。它是个人文件报告，与管理员的脱敏部署报告不同，请在分享前自行查看。

本次升级不新增切片引擎、固件兼容性、多色启动或双喷嘴打印支持。
