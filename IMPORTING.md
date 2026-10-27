# Sliced 3MF intake / 切片文件上传

0.1.1-beta · [English](README.md) · [简体中文](README.zh-CN.md)

## English

1. In Bambu Studio, choose the actual printer, nozzle, build plate and material, then slice the intended plates.
2. Export the **sliced plate 3MF**. Saving an unsliced project is insufficient. Menu wording may vary with Studio version.
3. In LayerHub, choose **Dashboard → New job / Upload sliced file** and select or drop the file.
4. LayerHub checks actual slice metadata. A compact verified-file card appears inline; expand it to view plates, color codes, material, estimates and file parameters. There is no import-purpose dialog or model editor.
5. Confirm device and actual spool, then add the job to its queue. Print preparation still checks the selected plate and feed mapping before transfer/start.

Only `.3mf`, including `.gcode.3mf`, is accepted, up to 500 MB or the smaller configured limit. Raw `.gcode`, STL and unsliced model projects are rejected by both UI and backend. Filename changes do not bypass content checks. A sliced export may also contain geometry, which Beta ignores rather than loading or editing.

Inspection is authenticated and CSRF protected, temporary uploads are cleaned, and no jobs, reservations, commands or emails are created. Known single-plate time and total grams are rounded up into editable queue estimates. Multi-plate files require an explicit plate selection. Selecting one plate prefills that plate’s estimates; selecting several opens the batch review. Missing values remain blank. See [multi-plate intake](MULTIPLATE.md). Stock is reserved only when you explicitly create the job and actual consumption is settled separately.

LayerHub retains the file's original sliced paths/process. Parameter references are display-only; LayerHub does not modify or regenerate them. Successful inspection is not device compatibility approval: existing machine, nozzle, plate, material, AMS and firmware authorization checks remain. No new multicolor/dual-nozzle support is implied.

The verified-file details can export a local JSON report. It contains the filename and file parameters, so review it before sharing. Cancelled, failed or stale inspections do not replace a previously selected file or create a task. Beta's removed model APIs return HTTP 410.

## 简体中文

1. 在 Bambu Studio 中选择实际机型、喷嘴、打印板与材质，完成需要打印的盘的切片。
2. 导出**切片盘 3MF**，不能只保存未切片工程；具体菜单名称可能随 Studio 版本变化。
3. 在 LayerHub 的**协作看板 → 新建任务 / 上传切片文件**选择或拖入文件。
4. 系统校验实际切片信息，通过后直接显示紧凑的“已校验”文件卡；展开可看各盘颜色代码、材质、用时、用量与文件参数，不再选择导入用途，也没有模型编辑入口。
5. 确认设备与实际料卷，加入对应队列；打印前准备继续核对具体盘和供料映射，然后传输、启动。

只接受 `.3mf`，包括 `.gcode.3mf`，最大 500 MB，配置更小限制时以服务器为准。界面和后端都拒绝原始 `.gcode`、STL、未切片模型工程，重命名不能绕过。有效切片包中同时含有的几何内容会被忽略，不加载或编辑。

检查需要登录与会话校验，临时上传会清理，不创建任务、库存预留、设备指令或邮件。单盘已知时长与总用量向上取整后预填，可修改；多盘文件必须明确勾选：选一盘就预填该盘数据，选多盘进入逐盘审核；缺失项仍留空。参见[多盘导入说明](MULTIPLATE.md)。用户明确创建任务后才预留库存，实际消耗另外结算。

原切片路径和工艺保持，参数仅供显示核对，不被 LayerHub 修改或重新生成。文件通过校验不等于设备兼容批准，机型、喷嘴、板型、材质、AMS 和固件授权等校验仍保留，不新增多色或双喷嘴打印支持。

文件详情可下载本地 JSON 报告，包含文件名与参数，分享前请自行查看。取消、失败或迟到的检查不会替换之前选定的文件或创建任务。已经移除的模型接口返回 HTTP 410。
