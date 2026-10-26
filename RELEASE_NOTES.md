# LayerHub 0.3.0-alpha · Deployment diagnostics / 部署与连接诊断

Inspect a new deployment before starting it with `layerhub --doctor`, optionally using `--json`, `--language=zh-CN` and the explicit `--probe-write` temporary-file test. The default inspection does not create a workspace, open SQLite, run an engine or make network connections.

Administrators now have a System diagnostics page. Browser network, the independent cloud gateway marker, the NAS application and existing printer telemetry are evaluated separately. Unknown or simulated status is not presented as validated hardware connectivity. Results expire, and existing authorization restrictions/alerts have specific guidance. Reports omit addresses, paths, credentials, names, filenames and raw errors, including when the NAS path fails.

新增启动前本地自检、管理员系统诊断和脱敏报告。浏览器、云网关、NAS 与打印机分段展示，过期或未知状态不会继续显示正常。检查不启动打印、不执行切片、不发送设备指令或测试邮件；目录写入测试需明确加 --probe-write。

- [Diagnostics / 诊断说明](https://github.com/Tuang-Gaashuan/LayerHub/blob/main/DIAGNOSTICS.md)
- [English deployment](https://github.com/Tuang-Gaashuan/LayerHub/blob/main/DEPLOYMENT.md)
- [中文部署](https://github.com/Tuang-Gaashuan/LayerHub/blob/main/01-部署指导-云服务器-NAS-打印机.md)
- [Validation / 验证范围](https://github.com/Tuang-Gaashuan/LayerHub/blob/main/validation/README.md)

Go tests/vet, Windows/Linux builds and 183 frontend/geometry/language tests are covered by release validation. Existing printer compatibility boundaries remain: real-print evidence is concentrated on the attended X2D single-color PETG Basic setup. Optional components being discoverable does not prove actual slicing, video, SMTP delivery or control authorization. Docker remains statically checked only.

Download the platform package and SHA256SUMS, or build matching source. Original project code and artwork are AGPL-3.0-only; third-party licenses are retained. New installations default to offline mode with print start disabled. Existing release assets remain unchanged.
