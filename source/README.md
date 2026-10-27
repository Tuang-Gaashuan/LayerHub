# LayerHub source

[简体中文](README.zh-CN.md) · [Deployment](../DEPLOYMENT.md)

Go HTTP + SQLite + native JavaScript. Original code is AGPL-3.0-only; third-party notices are retained.

## Build and run

Go **1.27+** is required. Dependencies are included in vendor/.

```sh
go test -mod=vendor ./...
go build -mod=vendor -trimpath -o bin/layerhub ./cmd/server
python3 tools/make-source.py
LAYERHUB_ADDRESS=127.0.0.1:5181 LAYERHUB_DATA_DIR=data LAYERHUB_WEB_DIR=web LAYERHUB_DEVICE_MODE=offline LAYERHUB_ENABLE_PRINT_START=0 ./bin/layerhub
```

On Windows build `bin/layerhub.exe`, set the same environment variables in PowerShell and run it. The app does not automatically load .env. First-run credentials are written to data/bootstrap-admin.txt; restarts preserve existing passwords. The first account is the sole workspace owner.

## Tests

```sh
go test -mod=vendor ./...
go vet -mod=vendor ./...
npm ci --prefix qa/frontend-deps --ignore-scripts
node --test qa/frontend.test.cjs qa/i18n.test.cjs
```

Tests use disposable workspaces and fake devices. Beta tests verify sliced 3MF intake and retired editor APIs; no slicing engine is used. Never supply a production data directory to tests.

## Corresponding source

```sh
python3 tools/make-source.py
```

The explicit allowlist builds web/source/layerhub-source.tar.gz, including vendor/ but excluding runtime data, node_modules and the archive itself. A complete distribution already contains it and the license files. Modified hosted versions must update this download and the version shown by web/source-offer.html. The source/license page is accessible from the footer and personal account.

Interface languages and translator workflow: [I18N.md](I18N.md). Compile/check catalog changes with `node qa/build-locales.cjs` and `node qa/build-locales.cjs --check` after installing the test dependencies.

Deployment checks / 部署自检：see [Diagnostics / 系统诊断](../DIAGNOSTICS.md). Use `--doctor` before startup; directory write probes require `--probe-write`.
