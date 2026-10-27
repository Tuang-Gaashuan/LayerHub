# Beta acceptance / Beta 验收

Use disposable offline workspaces before production. Record version, OS/architecture, browser and device firmware/modules when relevant. Never publish databases, backups, access codes, credentials or recipient addresses.

| Check | Expected result |
|---|---|
| Fresh startup/restart | Random workspace owner, offline mode, print start off; account persistence |
| Team/roles/sessions | Owner/admin/member boundaries, rename, disable, session isolation and audit records |
| Removed workbench | No editor navigation/assets, old APIs 410, old bookmarks return to dashboard, no engine worker |
| File intake | Actual sliced 3MF only; STL, raw G-code, renamed invalid or unsliced project rejected before job/reservation |
| Preparation | Colors and plate metadata readable; single-plate estimates rounded, multi-plate/unknown estimates blank; explicit spool choice |
| Queues/inventory | Per-printer order, shared/private stock isolation, reservations separate from actual usage |
| Notifications | Owner-specific, deduplicated start/finish events, optional verified email; device finish independent of collection |
| Backup/restore | Owner-only, offline restore; core and legacy alpha rows retained, restored devices disabled |
| Gateway/diagnostics | Private loopback tunnel, HTTPS, fresh/unknown evidence separation, anonymous diagnostic denial, redacted export |
| Attended hardware last | Fresh telemetry first, then low-impact controls, finally a single explicitly authorized print |

Build with Go 1.27 and run `go test -mod=vendor ./...` / `go vet -mod=vendor ./...` in source. Install pinned frontend dependencies using `npm ci --prefix source/qa/frontend-deps --ignore-scripts`, then run `node source/qa/build-locales.cjs --check` and `node --test source/qa/frontend.test.cjs source/qa/i18n.test.cjs`.

Actual-binary checks: `tools/smoke.py`, `tools/doctor-smoke.py`, `tools/file-intake-smoke.py`, each with `--binary` and `--report` (`--web source/web` for service checks). They use disposable workspaces and no real printer or SMTP delivery. Docker runtime and fresh two-host OS installation remain separately unverified.

简体中文：先在独立离线目录验证全新启动、角色会话、旧工作台移除、切片 3MF 白名单、颜色与预计值、队列和库存隔离、通知去重与归属、备份恢复（包括旧 Alpha 工程数据）、HTTPS 与诊断。无真机时不要把遥测、控制或真实打印标为通过。真实邮件只使用同意接收的测试者；设备完工、人工取件与用量结算分别验收。迁移见 [Beta 迁移说明](BETA_MIGRATION.md)。
