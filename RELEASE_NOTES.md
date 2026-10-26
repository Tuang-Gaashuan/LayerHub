# LayerHub 0.2.0-alpha · English interface / 英文界面支持

This release adds English and Simplified Chinese throughout the application: sign-in, dashboard, printer queues, inventory, printer controls, security settings and slicing. Language is available before sign-in and in Personal account. Account preferences determine future verification and job/material/printer emails; independent NAS outage notices are bilingual.

Model names, filenames and member data remain unchanged. A language change waits for the workbench draft save to be acknowledged before reloading. Failed or uncertain saves preserve the current page. Protocol keys, numerical controls and material IDs retain their existing behavior.

本版补齐软件中英文界面，覆盖登录、看板、队列、耗材、设备控制、安全设置与切片；账户语言用于后续邮件。语言切换前确认工作台草稿保存，失败时保留页面。文件名、用户内容、控制参数和耗材映射保持。

- [English deployment](https://github.com/Tuang-Gaashuan/LayerHub/blob/main/DEPLOYMENT.md)
- [中文入口](https://github.com/Tuang-Gaashuan/LayerHub/blob/main/README.zh-CN.md)
- [Language settings / 语言设置](https://github.com/Tuang-Gaashuan/LayerHub/blob/main/LANGUAGES.md)
- [Validation / 验证范围](https://github.com/Tuang-Gaashuan/LayerHub/blob/main/validation/README.md)

Download the Windows/Linux amd64 archive plus SHA256SUMS, or build the matching source. Original project code and artwork are AGPL-3.0-only; third-party notices are retained. Packages contain no production credentials, private data, slicer engine binary or networking plugin. New installations stay offline with print start disabled.

Go tests/vet and Windows/Linux builds passed; 178 frontend/geometry/language tests passed. English hardware controls were exercised against fake devices only. The existing attended X2D print scope is unchanged. Docker remains statically checked only; independent fresh deployment acceptance is still required.
