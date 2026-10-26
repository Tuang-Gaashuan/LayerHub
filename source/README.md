# LayerHub source

[简体中文](README.zh-CN.md) · [Deployment](../DEPLOYMENT.md)

Go HTTP + SQLite + native JavaScript/Three.js. Original code is AGPL-3.0-only; third-party notices are retained.

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
node --test qa/frontend.test.cjs qa/slicer-gizmo.test.mjs qa/face-placement.test.mjs qa/box-selection.test.mjs
```

Tests use disposable workspaces and fake devices. Optional real-engine experiments skip unless explicitly configured; skipping is not proof of hardware compatibility. Never supply a production data directory to tests.

## Corresponding source

```sh
python3 tools/make-source.py
```

The explicit allowlist builds web/source/layerhub-source.tar.gz, including vendor/ but excluding runtime data, node_modules and the archive itself. A complete distribution already contains it and the license files. Modified hosted versions must update this download and the version shown by web/source-offer.html. The source/license page is accessible from the footer and personal account.
