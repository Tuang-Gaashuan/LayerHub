# Security / 安全与测试反馈

Use a separate test installation for this alpha. Public reports should contain only redacted model/firmware, release version, error codes and reproduction steps. Never attach a database, full backup, private model, raw telemetry, email address, device code, token, SSH key or session cookie.

Each deployment generates its own owner password and data encryption key. Keep application code and its source download separate from `data/`. Maintain HTTPS, restricted OS accounts and disabled print-start permissions until an onsite operator completes acceptance checks. Backups contain private uploads and credentials and must remain private.

For vulnerabilities, use an existing private contact channel with the maintainer rather than a public issue; a dedicated reporting contact is not yet available. No automatic diagnostic upload to the maintainer is included.

## 简体中文

公开讨论请只提交脱敏的机型、固件、部署版本、错误代码和复现步骤。不要提交数据库、完整备份、邮箱、访问码、令牌、SSH 私钥、Cookie、个人模型或原始遥测。

本包没有预置管理员密码、邮箱授权码、设备访问码或 SSH 密钥。每个部署独立生成首次所有者密码与 master.key。部署者负责自己的系统账户与 HTTPS 证书；正式接入时保留默认关闭的远程启动开关，逐项验证后再由现场人员开启。

代码与源码下载路径必须与 data/ 完全分离。全量备份包含私人模型、密码哈希与设备密钥，不得作为公开附件。分享工程只提供作者主动发布的快照。

alpha 版本先用于独立测试环境。报告漏洞时通过你与项目维护者已有的私下联系渠道提供脱敏细节；仓库正式发布后由维护者补充专用联系方式。当前包没有自动向维护者发送诊断数据的功能。
