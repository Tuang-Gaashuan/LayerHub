# LayerHub 0.4.0-alpha · File intake and print preparation / 文件导入与打印准备

This bundled release improves the route from Studio files to an editable project or a print queue. Both file entries now inspect actual 3MF contents. A file may offer editing, an existing slice, both, or neither; unsupported geometry does not hide a valid slice.

1. Explicit edit/print routes with cancellation and late-response isolation.
2. Original printer, nozzle, plate, material, color, layer and infill references, compared with workbench defaults before geometry import.
3. Per-plate material/color codes, time and weight preview.
4. Rounded single-plate estimates in queue preparation; blank fields for unknown or multi-plate estimates instead of guessed defaults.
5. Local personal-file inspection report export with clear sharing scope.
6. Backend rejection of unsliced/invalid 3MF before queue creation or inventory reservation, bounded temporary inspection uploads and archive-link rejection.

一次合并升级：按内容识别 3MF、保留切片或编辑几何的双入口、原工程参数对照、逐盘颜色与预计值预览、单盘预计值预填、检查报告导出，以及后端拒绝未切片工程进入打印队列。中英文界面、导入指南、部署包与源码同时更新。

Geometry import still repacks objects, discards original X/Y placement and uses LayerHub process settings. References are not inherited configuration. Single-color/single-nozzle X2D hardware scope and existing print-start/authorization/freshness guards remain. No real print or SMTP delivery is needed for these file-workflow checks. Docker runtime and fresh two-host OS installation remain outside this validation.

- [Import guide / 文件导入说明](https://github.com/Tuang-Gaashuan/LayerHub/blob/main/IMPORTING.md)
- [English deployment](https://github.com/Tuang-Gaashuan/LayerHub/blob/main/DEPLOYMENT.md)
- [中文部署](https://github.com/Tuang-Gaashuan/LayerHub/blob/main/01-部署指导-云服务器-NAS-打印机.md)
- [Validation / 验证范围](https://github.com/Tuang-Gaashuan/LayerHub/blob/main/validation/README.md)

Download the platform package and SHA256SUMS, or build matching source. Original code and artwork are AGPL-3.0-only; third-party licenses remain. New installations default to offline mode and print start disabled. Existing release assets are unchanged.
