# LayerHub

[English](README.md) · [中文部署指导](../01-部署指导-云服务器-NAS-打印机.md)

多人打印协作服务，Go HTTP + SQLite + 原生 JavaScript/Three.js。AGPL-3.0-only；字体、Three.js、Go 依赖及来源于 Bambu Studio 的材料配置保留各自通知。

## 从源码启动

需要 Go **1.27 或更高版本**。Go 依赖已包含在 vendor/，构建不需要访问模块仓库。

```sh
go test -mod=vendor ./...
go build -mod=vendor -trimpath -o bin/layerhub ./cmd/server
python3 tools/make-source.py
LAYERHUB_ADDRESS=127.0.0.1:5181 LAYERHUB_DATA_DIR=data LAYERHUB_WEB_DIR=web LAYERHUB_DEVICE_MODE=offline LAYERHUB_ENABLE_PRINT_START=0 ./bin/layerhub
```

Windows PowerShell：

```powershell
go test -mod=vendor ./...
go build -mod=vendor -trimpath -o bin/layerhub.exe ./cmd/server
python tools/make-source.py
$env:LAYERHUB_ADDRESS='127.0.0.1:5181'
$env:LAYERHUB_DEVICE_MODE='offline'
$env:LAYERHUB_ENABLE_PRINT_START='0'
.\bin\layerhub.exe
```

首次凭据写入 `data/bootstrap-admin.txt`，不会在重启时重置。首次用户是唯一空间所有者。默认设备离线，不连接或启动任何打印机。环境变量模板见交付包 `deploy/nas/layerhub.env.example`；Go 程序不自动加载 .env。

## 测试

```sh
go test -mod=vendor ./...
go vet -mod=vendor ./...
npm ci --prefix qa/frontend-deps --ignore-scripts
node --test qa/frontend.test.cjs qa/i18n.test.cjs qa/slicer-gizmo.test.mjs qa/face-placement.test.mjs qa/box-selection.test.mjs
```

部分真实引擎实验默认跳过：它们需要部署者独立安装固定版本 Studio 及配置，不能将跳过当作实机通过。Go 测试使用临时数据和假设备；不要给测试传入真实 data/。

## 对应源码下载

```sh
python3 tools/make-source.py
```

它在 `web/source/layerhub-source.tar.gz` 生成明确白名单的源码包，包含 vendor/，排除运行数据、node_modules 及下载包自身。完整交付包已生成此文件及许可证副本。自行修改并发布时，重新生成对应源码，并更新 `web/source-offer.html` 的版本；该页面可从网页页脚及个人账户访问。

发布部署流程见根目录中英文指南。完整仓库保留父级目录结构，根目录构建工具会同时生成 Linux/Windows 程序和对应源码。没有预置生产服务器地址、账号或凭据。

界面支持简体中文与 English，登录页和个人账户均可切换。语言包开发流程见 [I18N.md](I18N.md)。修改文案后执行 `node qa/build-locales.cjs`，CI 使用 `--check` 检查漏翻和生成文件。

Deployment checks / 部署自检：see [Diagnostics / 系统诊断](../DIAGNOSTICS.md). Use `--doctor` before startup; directory write probes require `--probe-write`.
