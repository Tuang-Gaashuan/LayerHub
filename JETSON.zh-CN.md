# Jetson Orin Nano 8GB · Ubuntu 22.04 ARM64 验证准备

[English](JETSON.md)

本目录对应 Linux ARM64（aarch64）验证包，不是 linux-amd64 包。当前只完成交叉编译与包结构/架构检查；Jetson 实际运行、性能、摄像头和打印机连接需后续验收。保持 NVIDIA 提供的 Jetson 系统与驱动；无需为 LayerHub 更换内核或安装 CUDA。

NVIDIA 的 JetPack 6.1 文档说明该平台采用 Ubuntu 22.04 根文件系统并支持 Orin 系列：[官方说明](https://developer.nvidia.com/embedded/jetpack-sdk-61)。这是平台依据，不是建议更改现有 JetPack 版本。LayerHub 核心使用 CPU 与 Go/SQLite，不包含 GPU 推理或 Jetson 硬件视频加速实现。

## 包的位置与内容

本地 `local-packages/jetson-arm64/` 包含 `LayerHub-0.1.4-beta-jetson-arm64-preview.tar.gz`、`SHA256SUMS` 和构建报告。包含 ARM64 服务程序、可选邮件工作器、网页、对应源码、部署脚本及图文指南，不包含生产数据库、设备访问码或SMTP凭据。

现有正式 v0.1.3-beta 下载附件和更新工具仍是 AMD64 范围。**Jetson 先使用本验证包手动部署，不运行旧版 NAS-Update.sh / 下载更新工具。** 验证包没有新增 ARM64 自动更新支持，不覆盖正式发布附件。

## 1. 连接前准备

在 Jetson 执行：

```sh
uname -m
cat /etc/os-release
free -h
df -h .
```

架构应为 `aarch64`，系统为 Ubuntu 22.04。记录 JetPack/L4T 版本及存储介质；如已安装，可用 `head -n 1 /etc/nv_tegra_release`。不要把这些检查当作已有性能结果。

后续通过已有SSH账号连接；需要新开SSH时可在Jetson本机安装并启用 `openssh-server`。仅在可信网络访问，不发布账号密码或密钥。

```sh
sudo apt update
sudo apt install -y python3 ca-certificates curl openssh-server
sudo systemctl enable --now ssh
```

视频验证按需安装发行版的 FFmpeg，并检查 `ffmpeg -version`；先采用常规软件路径，不假定当前程序已使用Jetson GPU。避免替换NVIDIA驱动或现有多媒体环境。

## 2. 解压与只读预检

将压缩包与校验文件放到同一目录：

```sh
sha256sum -c SHA256SUMS
tar -xzf LayerHub-0.1.4-beta-jetson-arm64-preview.tar.gz
cd LayerHub-0.1.4-beta-jetson-arm64-preview
sh deploy/nas/install.sh --check
./bin/layerhub --doctor --json
```

`--check` 不安装、不创建账号、不启动服务；它检查主机和二进制都是64位Linux、架构匹配且没有动态加载器依赖。`--doctor` 不启动后台或连接打印机；未初始化的数据目录应按“全新目录”解释诊断结果。

## 3. 先离线启动

```sh
sh start.sh
```

默认使用 `127.0.0.1:5181`、当前目录下的独立 `data/`、offline模式和关闭远程打印启动。在另一终端检查：

```sh
curl --noproxy '*' http://127.0.0.1:5181/healthz
```

从电脑访问时使用SSH转发：

```sh
ssh -N -L 5182:127.0.0.1:5181 jetsonuser@JETSON_IP
```

浏览器打开 `http://127.0.0.1:5182/`。初始所有者凭据在验证包目录的 `data/bootstrap-admin.txt`，自行读取，不发在聊天。终端 Ctrl+C 可停止便携服务。

## 4. 新机器的 systemd 安装

离线试用正常后，先停掉便携服务。只对未安装 LayerHub 的主机执行：

```sh
sudo sh deploy/nas/install.sh
sudo systemctl status layerhub --no-pager
curl --noproxy '*' http://127.0.0.1:5181/healthz
```

安装器按 `VERSION` 和架构建立发布目录，默认数据为 `/var/lib/layerhub/data`，配置为 `/etc/layerhub/layerhub.env`。这是一个新的工作区，不自动迁移便携数据。发现已有账号、服务或数据时拒绝安装；不要通过删除检测项来绕过。Ubuntu22.04的原生systemd版本需在机器上验证。ARM64更新暂采用另行准备的手动升级方案。

## 5. 后续验收安排

1. 离线功能：登录、SQLite持久化、重启、中文/英文、后台解析和无打印任务的耗材/队列演示。
2. 性能：记录空闲10分钟的CPU/RSS/温度，真实切片文件上传与解析用时、峰值内存、网页API延迟；固定文件、网络与采样条件，再与Dell NAS比较，不预先承诺快多少。
3. 摄像头：FFmpeg可执行性、A1/X2D画面实际帧率、CPU占用与断流恢复；不将机器原生低帧率算作Jetson性能问题。
4. 云端通道：独立测试SSH隧道与断线重连。云端只转发到一个明确的业务后端；不要让两个主机共享或复制运行中的同一数据库。
5. 最后再接打印机：先状态读取，保持远程启动关闭；现场确认和明确授权后才做控制、加热、运动、供料及打印。不得和现有NAS同时成为同一任务的控制端。

## 从源码重建

开发电脑安装 Go 1.27+ / Python 3 后，在仓库根目录运行：

```sh
python3 tools/build-arm64-preview.py --go go
```

使用 vendor 离线构建，固定 `GOOS=linux GOARCH=arm64 GOARM64=v8.0 CGO_ENABLED=0`。无需Jetson本机编译器、CUDA或C版SQLite。静态ELF检查通过不等于已经在Orin上运行通过。
