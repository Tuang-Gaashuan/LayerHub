# Deployment checks and connection diagnostics / 部署自检与连接诊断

## English

Administrators can open **System diagnostics** in the sidebar. The page reads existing state and makes same-origin GET requests only. It does not send printer commands, start a print, contact SMTP, change settings, or retry an unconfirmed operation.

### Read the chain separately

| Segment | What a positive result establishes | What it does not establish |
|---|---|---|
| Browser network | The browser does not report being offline | Internet or NAS reachability |
| Cloud gateway | This origin returned the independent `layerhub-gateway` health marker | A working reverse tunnel or NAS application |
| NAS application | An authenticated diagnostic response arrived | All optional components or device controls working |
| Printers | At least one enabled live device has telemetry less than 60 seconds old | Every printer online, or authorization to start printing |

Use **All checks** for component detail and individual printer IDs. Missing optional components are distinguished from errors. Simulated telemetry is never counted as a real printer connection. Existing device authorization restrictions and alerts are shown separately from connectivity.

Results are snapshots. After 60 seconds, or when the browser reports being offline, old results stop appearing healthy until checks are run again. There is no automatic diagnostic polling or command retry. Ordinary workspace status refresh continues independently.

A missing gateway marker is **not proof of an outage**: local-only access and older gateway templates do not provide it. With the marker available but the application unavailable, inspect the reverse tunnel, NAS service, power and sleep state; the page cannot tell which of these failed or whether a printer stopped.

The cookie check distinguishes a static configuration switch from the current request policy. Direct TLS and the existing trusted loopback proxy policy can enable Secure cookies even when the switch is off. A forwarded header from a non-loopback peer is not accepted as evidence. Local CLI checks cannot establish the public request policy.

### Install the independent gateway marker

New deployments include `/gateway-healthz` in `deploy/cloud/nginx.conf.example`. Existing deployments can add the following inside the LayerHub HTTPS `server` block, without changing its existing certificates, ports, relay destination or access restrictions:

```nginx
location = /gateway-healthz {
    default_type application/json;
    add_header Cache-Control no-store always;
    return 200 '{"ok":true,"role":"layerhub-gateway","schema":1}';
}
```

Validate with `sudo nginx -t` before `sudo systemctl reload nginx`. This endpoint runs in Nginx, independently of the NAS tunnel. Do not proxy it to the application. A matching role/schema is required; a generic HTTP 200 page does not count.

### If the application cannot start

Run the binary in the extracted installation directory, as the **actual service account** and with the same environment settings:

```powershell
# Windows portable package
.\bin\layerhub.exe --doctor
.\bin\layerhub.exe --doctor --json
```

```sh
# Linux portable package
./bin/layerhub --doctor
./bin/layerhub --doctor --json
```

Use `--language=zh-CN` for Chinese output. Source-build binary locations differ; the configuration variables are the same. The program does not automatically load `.env` or `/etc/layerhub/layerhub.env`. A check run as root or with a different working directory/environment does not establish that the service user can access its files. Review your service's existing environment configuration locally without sharing its contents.

`--doctor` does **not** start an HTTP listener, open or initialize SQLite, start background workers, or make network connections. It checks listener/mode configuration, existing files, optional FFmpeg discovery, basic mail configuration and the SQLite file header. A readable header is not a database integrity check. An executable found on disk is not proof that its runtime libraries work.

For an explicit directory write test:

```sh
./bin/layerhub --doctor --probe-write --json
```

This creates one uniquely named `.layerhub-doctor-*` temporary file in the **already existing** data directory, writes and reads it, then removes it. It never creates a workspace or edits existing data. Without this flag, write permission remains **not verified**. If cleanup fails, the check reports an error; any leftover file contains only a fixed diagnostic marker.

Exit codes: **0** = no checks reported `error` (warnings and unknowns may remain); **1** = at least one `error`; **2** = invalid command-line usage. JSON `ok: true` means a report was produced, not that the entire deployment is healthy. Interpret each check's status.

### Share a useful report

**Download redacted report** exports the snapshot, including a failed application check when the gateway alone responded. Addresses, serial numbers, user names, model filenames, absolute paths, tokens, recipient addresses and raw exception strings are excluded by the report format. The `stale` flag is true when application evidence is missing, expired, or the browser is offline; gateway evidence is listed separately.

Report a problem with the version, platform, relevant check IDs/statuses and your reproduction steps. The report does not prove SMTP delivery, real engine output or print quality; those need separate acceptance tests. Do not attach environment files, databases or full machine backups.

## 简体中文

管理员可在侧栏打开**系统诊断**。页面仅通过同源 GET 请求读取状态，不发送打印机指令，不启动打印，不连接 SMTP，不修改设置，也不重试未确认操作。

**分段看链路：**浏览器“在线”不等于公网可达；网关标记响应不等于 NAS 正常；业务服务返回诊断不等于可选组件全部就绪；实机遥测新鲜不等于控制和打印已获授权。逐台设备结果在“全部检查”中查看，模拟设备不会计作真实连接；已有授权限制与设备告警另行提示。

检查结果是快照，超过 60 秒或浏览器离线后，不再沿用绿色正常状态。诊断不自动轮询；工作区原有状态刷新保持。没有网关标记时，可能是本地直连、旧模板或入口不可达，不能直接断言云端掉线。网关可达但业务未确认时，需要排查隧道、NAS 服务、供电和挂起状态，不能据此断言打印停止。

Cookie 检查区分静态开关与当前请求策略：直接 TLS 或已有可信回环代理策略可启用 Secure Cookie；来自非回环来源的转发头不作为有效证据。本地命令行不能确认公网请求的实际策略。

**升级旧网关：**在已有 LayerHub HTTPS `server` 内添加上方 `/gateway-healthz` 配置，保留当前证书、端口、代理目的地址和访问限制。先 `sudo nginx -t`，通过后 `sudo systemctl reload nginx`。此接口由 Nginx 独立返回，不要转发给 NAS；普通 200 页面不能冒充健康标记。

**服务启动不了时：**在安装目录，以实际服务账户、实际配置环境运行：

```powershell
# Windows
.\bin\layerhub.exe --doctor --language=zh-CN
.\bin\layerhub.exe --doctor --json --language=zh-CN
```

```sh
# Linux
./bin/layerhub --doctor --language=zh-CN
./bin/layerhub --doctor --json --language=zh-CN
```

程序不会自动读取 `.env` 或 `/etc/layerhub/layerhub.env`。使用 root、其他工作目录或不同环境运行，不能证明实际服务账户具有相同权限。请在本机核对服务配置，不要把环境文件内容发到公开反馈中。

自检不监听端口，不打开或初始化数据库，不启动后台任务，不执行引擎，也不建立网络连接。它检查配置、资源文件、有限大小的配置 JSON、可执行文件发现、可选 FFmpeg、邮件基础配置和 SQLite 文件头；文件头可读不代表数据库完整，引擎文件存在也不代表运行库齐全。

只有明确添加 **`--probe-write`**，才在已存在的数据目录创建、写入、读回并删除一个独立临时文件。不会创建工作区或修改业务文件；不加参数时，写入权限标为“尚未验证”。清理失败会报错，残留文件仅含固定自检标记。

退出码 **0** 表示没有 `error`，仍可能有警告或未知项；**1** 表示存在错误；**2** 表示命令用法无效。JSON 的 `ok: true` 仅表示成功生成报告，不表示整套系统通过验收。

**下载脱敏报告**支持导出链路失败结果，不包含 IP、序列号、用户名、文件名、绝对路径、令牌、邮箱及原始异常。未取得业务响应、结果过期或浏览器离线时，报告标有 `stale: true`，网关结果单列。反馈时提供版本、平台、检查项编号/状态与复现步骤即可；真实邮件投递和打印质量仍需独立验证。
