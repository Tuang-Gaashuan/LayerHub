# LayerHub 0.1.0-alpha · 外部部署测试包

[English](README.md) · [GitHub Releases](https://github.com/Tuang-Gaashuan/LayerHub/releases)

多人共用打印机的网页工作空间：单机队列、逐卷耗材、私有工程、在线切片、团队工程快照及通知。

本包是独立的新部署，不接入开发者原有服务器或打印机。未包含账号、SSH 密钥、邮箱授权码、数据库、私人模型或切片引擎二进制。第一次启动会生成自己的所有者密码。

## 先从这里开始

1. **只看界面/验证安装**：解压 `packages/LayerHub-0.1.0-alpha-windows-amd64.zip` 或 Linux 包，运行其中的启动脚本。默认 offline、远程启动关闭。
2. **部署真实架构**：阅读 [云服务器 → NAS → 打印机](01-部署指导-云服务器-NAS-打印机.md)，先通 NAS，再通云网关，最后只读接入打印机。
3. **准备切片**：引擎不随包分发，按 [独立切片引擎准备](deploy/slicer/README.md) 安装固定版本。没有引擎仍可登录、管理队列/耗材、导入和查看模型。
4. **提交结果**：按 [外部验收与反馈](02-外部验收与反馈.md) 记录结果。请勿公开备份和原始凭据。

## 目录

| 路径 | 内容 |
|---|---|
| `source/` | 可独立构建的 Go/前端源码、Go 依赖源码与测试 |
| `binaries/windows-amd64/` | Windows 服务端程序 |
| `binaries/linux-amd64/` | Linux NAS 服务端和可选云邮件工作器 |
| `packages/` | 便携 Windows、Linux 和对应源码压缩包 |
| `deploy/nas/` | 原生 systemd 服务、环境模板、主动 SSH 隧道 |
| `deploy/cloud/` | HTTPS 网关和受限 SSH 账号配置 |
| `deploy/docker/` | Dockerfile / Compose；默认仅绑定本机回环 |
| `deploy/mail/` | 可选 163 SMTP 通知工作器模板 |
| `deploy/slicer/` | 不包含引擎的准备工具和资源约束说明 |
| `tools/` | 构建、校验、打包工具 |
| `licenses/` | AGPL 及第三方依赖许可证归档 |
| `validation/` | 本次脱敏验证报告与文件清单 |

## 版本边界

- 提供 Windows amd64、Linux amd64。Linux 原生部署是完整链路的主要参考；Docker 基础模板未内置图形切片依赖，不宣称直接挂 AppImage 就能切片。
- 实机验证集中于 X2D / 0.4 mm / 单色 PETG Basic 的既有固件及模块组合。图鉴展示其他机型，不等于已经支持控制这些设备。
- 其他环境、固件与材料需要测试者分别登记。开启网络读取不自动获得打印授权；启动仍需要现场条件和用户确认。
- 同一个数据目录只能由一个服务实例使用。云端只做网关，不能再运行第二份应用并共用/复制同一实时数据库。

## 开源与对应源码

项目自身按 **AGPL-3.0-only** 发布，第三方组件保持原许可，见 [LICENSE](LICENSE) 与 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。源代码入口也出现在网页页脚和个人账户中，并提供此版本对应源码下载。修改后对外部署前应更新源码下载内容。

公开仓库保留完整目录结构；二进制和压缩包在 Releases 提供，也可按英文首页从源码构建。不要将运行后产生的 data/ 或整份机器备份提交到仓库。
