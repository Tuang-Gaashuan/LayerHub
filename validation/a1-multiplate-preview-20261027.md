# A1 / multi-plate Beta preview validation

Prepared 2026-10-27. Local preview only; no GitHub publication and no claim of A1 physical validation.

## Results / 结果

- `go test ./...` and `go vet ./...`: passed across application, device, mailer and updater packages.
- `node --test qa/frontend.test.cjs`: **89 passed, 0 failed**.
- Focused backend coverage: atomic reservations and rollback; concurrent identical imports; changed-payload rejection; user-scoped receipts; sparse plate IDs; shared-file deletion; plate/preparation mismatch; A1 external-spool preparation and AMS refusal; bounded PNG decoding; schema 4→5 migration and newer-version refusal; backup/restore of plate IDs, receipts and files.
- Actual Windows binary in temporary offline workspaces: multi-plate HTTP intake, receipt replay after deletion, source-file lifetime, sliced-only intake, fresh startup/restart, and diagnostic CLI checks passed. No live workspace was used.
- Browser checks: Chinese/English at 1440×1000 and Chinese at 390×844; batch and spool dialogs checked, no JavaScript errors or horizontal overflow. The confirmation button remains reachable by scrolling on mobile.
- No physical control commands, print starts, test notifications, production jobs, settlements or inventory deductions were issued by this validation.

## Limits / 尚待验证

A1 LAN/Developer Mode, camera and printing still require the actual device, its firmware/version and an on-site person. Multi-plate HTTP tests use synthetic sliced fixtures, including noncontiguous plate IDs 1 and 7; a real Bambu Studio multi-plate archive and individual physical plate starts remain acceptance work. Batch intake supports single-filament plates and does not authorize automatic successive prints.

Production was observed running another job during preparation, so the NAS application and cloud UI were not replaced. This preview requires a coordinated idle-window upgrade and a workspace backup. Schema 5 cannot be safely read by pre-feature binaries; rollback requires the matching pre-upgrade workspace backup.

中文：本地功能、故障边界、真实离线程序、中英文桌面和手机页面已验收；A1 实机及真实多盘文件打印待现场验收。本次没有部署或操作生产打印机。详见 [A1 接入准备](../A1_SETUP.md) 和 [多盘导入](../MULTIPLATE.md)。
