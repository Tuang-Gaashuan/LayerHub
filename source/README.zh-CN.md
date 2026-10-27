# LayerHub Beta 源码

[English](README.md) · [主项目说明](../README.zh-CN.md) · [部署指南](../01-部署指导-云服务器-NAS-打印机.md)

本主线为 v0.1.1-beta 起的团队打印管理版本，采用 Go、SQLite 与原生 JavaScript。成员在 Bambu Studio 切片后上传 3MF；模型工作台、Three.js 与在线切片引擎已移除。原项目代码采用 AGPL-3.0-only。

Go 1.27+，依赖已纳入 vendor。使用 `go test -mod=vendor ./...`、`go vet -mod=vendor ./...` 验证，`go build -mod=vendor -trimpath -o bin/layerhub ./cmd/server` 构建（Windows 使用 .exe）。首次运行默认离线并关闭打印启动，设置 LAYERHUB_DATA_DIR 与 LAYERHUB_WEB_DIR；环境文件不会被应用自动读取。

前端依赖：`npm ci --prefix qa/frontend-deps --ignore-scripts`。语言检查：`node qa/build-locales.cjs --check`。测试：`node --test qa/frontend.test.cjs qa/i18n.test.cjs`。

`python tools/make-source.py` 按白名单生成对应源码包；不得把生产账号、数据目录、私钥、上传或备份加入源码下载。完整分发包提供许可证和源码，修改部署后应更新对应包与源码页版本。Alpha → Beta 迁移详见 [迁移说明](../BETA_MIGRATION.md)。
