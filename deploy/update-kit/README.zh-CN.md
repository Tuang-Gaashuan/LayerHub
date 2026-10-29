# LayerHub 一键下载 / 更新工具包 · v0.1.2-beta

本工具包与 LayerHub v0.1.2-beta 一并发布。默认下载与更新 **Beta**，推荐配合 Bambu Studio。Windows 工具为独立程序，不需要额外安装 Python；NAS/云端限 Debian/Ubuntu 原生 systemd、Linux amd64，数据备份需要 Python 3。暂不支持 Docker、ARM、云端首次证书/隧道部署，或自动把 Alpha 数据迁移到 Beta。

## Windows：双击下载

- `Download-Beta-Windows.cmd`：自动查找 Beta 通道的新版本，下载、校验并解压到 `downloads`。进入解压目录后，按应用包说明启动 `start.cmd`。已有目录与运行数据不会被覆盖。
- `Download-Beta-NAS.cmd`：下载 NAS/Linux 包与 `.verified.json` 收据，保存在 `downloads`，适合拷贝到 NAS/云端离线更新。
- `Start.cmd`：打开菜单。高级用户也可用 `bin/layerhub-update-windows-amd64.exe download --help` 查看参数。

程序优先使用代理环境变量，否则读取 Windows 系统代理；不修改系统代理设置。下载经 HTTPS，核对 SHA256SUMS，并在 GitHub 提供附件摘要时再次核对。中断的文件保留为 `.part`，重新运行可尝试续传。工具不会执行下载包中的程序或脚本。

GitHub 对公共 API 有访问频率限制，共用代理出口也会共用额度。若遇到额度耗尽，工具会显示预计可重试时间；等待后重试，或在有可用连接的可信电脑下载 NAS 包再离线更新。不会把网络失败当作“已经是最新版”。

首次部署 NAS 仍需按应用包的部署指南配置账号、网络、HTTPS 与受限隧道。此工具包中的“更新”用于已经安装好的服务，不自动建立新的管理权限或打开防火墙端口。

## NAS：一键启动更新

将完整工具文件夹复制到 NAS。在 NAS 本机打开文件夹中的 `NAS-Update.sh`，选择在终端运行；也可以在工具文件夹中执行：

```sh
sudo sh NAS-Update.sh
```

工具识别 `/opt/layerhub/current`、`/etc/layerhub/layerhub.env` 和实际数据目录。未检测到新版本时直接结束，不停服务、不要求业务登录。确有新版本时：

1. 校验通道、附件与新程序版本；必要时要求输入空间所有者账号及密码，密码不回显、不保存。
2. 检查打印机在线、遥测新鲜且已空闲，拒绝进行中的打印、传输、启动、未确认指令、活动切片或待恢复操作。
3. 显示旧/新版本，由本机操作者输入 `yes` 确认。停止前后再次核对任务状态。
4. 停止 LayerHub，备份完整工作区、SQLite 一致性快照、master.key 与环境配置，备份仅 root 可读；不改 SSH、隧道、SMTP 或环境配置。
5. 切换至新的版本目录并启动，核对实际版本、登录、工作区及新鲜遥测。保留旧程序与备份。

备份位于 `/var/backups/layerhub-update/`。新程序启动前失败会恢复旧程序指向；**新程序一旦启动，可能已迁移或写入数据库，此后失败不盲目覆盖数据库或强切旧程序**，而是保留匹配程序、备份和现场供维护。没有自动“回滚数据”命令。

若原始所有者初始凭据已改过，工具会提示输入当前账号密码。自定义安装路径可使用 `--root`、`--env-file`、`--api-origin`，通过 `nas --help` 查看。数据、程序和下载缓存必须位于独立目录。

## 云端：NAS 成功后再更新界面

```sh
sudo sh Cloud-Update.sh
```

只更新既有 Nginx 网关的静态前端，不安装第二份数据库或应用。工具要求目标版本与 NAS 实际版本一致，保存原网关配置，保留端口、证书和安全规则。输入公网 HTTPS 根地址后，校验并重载 Nginx，核对实际页面摘要；失败时尝试恢复原网关配置，不回退 NAS 数据。

默认识别 `/etc/nginx/sites-available/layerhub` 或 `/etc/nginx/conf.d/layerhub.conf`；多份或不认识的配置会拒绝改写，可用 `--nginx-config` 明确指定。无额外 SSH 密钥分发或跨主机自动执行：NAS 与云端分别在本机运行各自入口。

## NAS 无法直接下载 GitHub 时

1. 在 Windows 双击 `Download-Beta-NAS.cmd`。
2. 把整个工具文件夹连同 `downloads` 拷贝到 NAS；离线包和 `.verified.json` 收据必须成对保留。
3. 在 NAS 本机运行 `sudo sh NAS-Update-Offline.sh`。
4. NAS 更新成功后，在云端复制同一工具文件夹，运行 `sudo sh Cloud-Update-Offline.sh`。

离线入口只检查本地传入的包，不查询 GitHub 是否有更新。收据核验传输完整性，**不是数字签名**；请确保包和收据来自你信任的下载设备，并通过可信方式传递。在线入口则通过 GitHub HTTPS 重新核对官方发布元数据。

## 先查看计划 / Alpha 通道

直接运行后台程序而不加 `--apply`，只做准备与预检，不停止服务：

```sh
sudo ./bin/layerhub-update-linux-amd64 nas --channel beta
sudo ./bin/layerhub-update-linux-amd64 cloud --channel beta
```

Alpha 可通过 `--channel alpha` 明确下载/更新。更新工具要求现有部署与所选通道一致，不执行 Alpha → Beta 切换，也不按版本数字大小把两个通道混在一起比较。使用已发布的固定版本可加 `--version v0.1.0-beta`。

## 本地源码与许可

`source/` 含工具本身与实际依赖的源码，支持 Go 1.27+ 离线构建：

```sh
cd source
go test -mod=vendor ./...
go build -mod=vendor -trimpath -buildvcs=false -o ../layerhub-update ./cmd/updater
```

原创代码遵循 AGPL-3.0-only。Go 运行时与 golang.org/x/sys 保留其许可证，见 `LICENSE` 与 `THIRD_PARTY_NOTICES.md`。工具不包含真实账号、SSH 私钥、SMTP 密码、生产数据库或运行配置。
