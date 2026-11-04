# 上传后直接开始 / Upload and start

选择已切片的 3MF、打印盘、目标打印机与实际料卷。设备空闲、状态新鲜且没有等待任务时，确认清空打印板后，单次确认会上传、为每盘创建任务、保存供料准备，再请求传输并开始首盘。无需返回队列再次准备。

多盘可选择先打印哪一盘；其余盘只入队，不会自动接续启动。也可选择“仅加入队列”。设备忙碌、有等待任务或启动条件不足时，界面说明原因，仅允许入队，不会插队。

提交前及启动前重新检查设备与队首。网络结果不明时保留原编号，只读核对不会再次启动；失败原因会持续显示。没有自动重发或启动下一盘。

Choose a sliced 3MF, plates, printer and loaded spools. When the printer is idle with fresh status and no waiting jobs, one confirmation uploads the file, creates one job per plate, saves preparation and requests transfer/start of the selected first plate. No second queue preparation step is needed.

For multiple plates, explicitly choose the first plate; the others stay queued and never start automatically. Queue-only remains available. Busy printers and existing queues are explained and never bypassed. Printer and queue checks run again before submission and start. Ambiguous results keep their original request ID; reading a receipt never restarts printing. Actual errors stay visible.


### X2D external-spool preparation

The confirmation screen selects a feed source for each plate independently of whether the printer has an AMS. A soft TPU plate defaults to the external spool. The external port is derived from the sliced physical nozzle mapping (`physical=1` -> `external:1` / virtual 254; `physical=0` -> `external` / virtual 255), and the inventory spool must be mounted at that exact port. Soft TPU must use the left direct-drive hotend.

Starting requires matching fresh nozzle specifications, external filament material and preset ID, and a fresh report that the corresponding extruder has filament from that external source. External filament must be loaded manually. TPU for AMS and TPU 90A are distinct presets. The software does not alter the printer profile, switch the material path, heat, or extrude while preparing a file. The flat mapping remains `-1`; the modern mapping carries the virtual source and slot 0. Legacy single-nozzle printer payloads are unchanged.

Implemented and tested locally on 2026-11-03. Real X2D external-spool print acceptance remains pending; no physical start was issued for this change.


### Illustrated member workflow / 图文使用流程

See [English](USAGE.md) / [简体中文](USAGE.zh-CN.md) for the six screenshots from Bambu Studio to the LayerHub confirmation screen. Current printer-report synchronization corrects material/color on the existing mounted spool and retains inventory grams. A physical spool replacement does not yet unmount the old record or assign a new one automatically.
