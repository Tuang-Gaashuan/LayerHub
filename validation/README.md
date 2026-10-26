# Release validation / 发布验证

0.1.0-alpha · 2026-10-26. Tests used the exported source or binaries rebuilt from it. No production application service was restarted, no live workspace was changed, no real printer was contacted and no email was sent for these release checks.

| Check | Result / scope |
|---|---|
| Exported Go source | `go test -mod=vendor ./...` and `go vet -mod=vendor ./...` passed on Windows; Windows/Linux amd64 builds passed |
| Frontend and geometry | 166 tests passed, zero failed |
| Windows fresh workspace | Passed: offline and print-start off, random owner credentials, private-file denial, source offer and restart persistence |
| Linux fresh workspace | Same checks passed as an unprivileged user |
| HTTPS and SSH gateway | Passed on isolated temporary listeners on one Linux host: template syntax/effective restrictions, allowed reverse forward, disallowed alternate port and Shell, verified TLS, Secure cookie, public internal-path denial and 503 after tunnel loss |
| Portable launchers | Windows command quoting/control characters checked; Linux shell syntax checked |
| Docker | YAML/defaults/volume/permissions checked statically; image build and container startup **not executed** because no Docker engine was available |
| Fresh OS installation | systemd installer is fresh-only; a clean two-host OS installation and every optional slicer GUI-library combination were **not executed** in this release check |
| Publication hygiene | Whitelist export; production host/address/code, personal paths, private keys/tokens and runtime-data checks; see publication-scan.json |

JSON reports in this folder contain no credentials or recipient addresses. Historical X2D real-print evidence describes an existing tested configuration; it is not a promise that every catalog model, firmware or newly installed slicing engine will work. Follow [independent acceptance tests](../TESTING.md) with your own hardware.

## 简体中文

本次使用导出的源码和重新构建的程序验证。Go 测试与 vet、166 项前端/几何测试、Windows/Linux 全新工作区与重启检查均通过。网关使用同一 Linux 主机上的独立临时监听器，验证了受限隧道、HTTPS、禁止 Shell/其他转发端口、私有路径拦截和断线 503；不代表已经替别人完成全新双主机部署。

没有重启现有生产应用，没有修改生产工作区，没有连接真实打印机或发邮件。Docker 仅静态检查；首次系统安装器和所有切片图形依赖组合仍需外部测试者按验收表实际验证。机型图鉴不等于控制兼容列表。
