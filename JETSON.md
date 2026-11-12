# Jetson Orin Nano 8GB · Ubuntu 22.04 ARM64 preview

[简体中文](JETSON.zh-CN.md)

This is a local Linux ARM64 (`aarch64`) preview. Cross-compilation and package/ELF checks do not establish real Jetson performance, camera compatibility or printer acceptance. Keep the installed NVIDIA kernel/drivers; LayerHub's CPU/Go/SQLite service does not require CUDA and does not implement Jetson GPU inference or hardware-video acceleration.

[NVIDIA JetPack 6.1 documentation](https://developer.nvidia.com/embedded/jetpack-sdk-61) identifies Ubuntu 22.04 as its root filesystem and Orin as supported. This is a platform reference, not a request to change the installed JetPack version.

## Build and transfer

From the repository root with Go 1.27+ and Python 3:

```sh
python3 tools/build-arm64-preview.py --go go
```

The vendor build uses `GOOS=linux GOARCH=arm64 GOARM64=v8.0 CGO_ENABLED=0`. Output is in `local-packages/jetson-arm64/`: a preview archive, SHA256SUMS and a build report. The archive contains the ARM64 server/mail worker, web assets, corresponding source and guides. No production database or credentials are included. Existing v0.1.3-beta published assets are not changed.

**The existing Download/Update Kit is AMD64-only. Do not use its NAS update/download entrypoints on Jetson. This preview uses manual deployment; ARM64 automatic upgrading is not implemented.**

## Check the Jetson

Record `uname -m` (expected `aarch64`), `/etc/os-release` (Ubuntu 22.04), `free -h`, storage availability and `/etc/nv_tegra_release` if present. Use an existing SSH account, or install/enable openssh-server locally. Dependencies for basic use: Python 3, ca-certificates and curl. FFmpeg is optional for camera tests; check its actual runtime before claiming camera support. Preserve NVIDIA drivers.

## Offline first

```sh
sha256sum -c SHA256SUMS
tar -xzf LayerHub-0.1.4-beta-jetson-arm64-preview.tar.gz
cd LayerHub-0.1.4-beta-jetson-arm64-preview
sh deploy/nas/install.sh --check
./bin/layerhub --doctor --json
sh start.sh
```

Preflight checks architecture/standalone ELF and does not install or start anything. Doctor does not start workers or contact devices. Portable startup defaults to loopback port 5181, a separate local data directory, offline devices and print start disabled. Check `/healthz` locally. From a computer, forward `ssh -N -L 5182:127.0.0.1:5181 jetsonuser@JETSON_IP`, then open `http://127.0.0.1:5182/`. Read the generated local `data/bootstrap-admin.txt` privately. Ctrl+C stops portable mode.

For a fresh systemd installation, stop portable mode first, then run `sudo sh deploy/nas/install.sh`. It creates a separate workspace at `/var/lib/layerhub/data` and refuses existing services/accounts/data. It does not migrate the portable workspace. Check `systemctl status layerhub` and the local health endpoint. Native Ubuntu 22.04 systemd behavior still needs device acceptance.

## Acceptance plan

- Offline startup, SQLite persistence/restart, language UI, background parsing and isolated queue/inventory demos.
- Measure 10-minute idle CPU/RSS/temperature, fixed-file upload/parse time and peak memory, and API latency under comparable conditions before comparing against the Dell NAS.
- Check FFmpeg and actual A1/X2D camera rates, CPU and reconnect behavior; distinguish printer-native frame-rate limits from server throughput.
- Test a separate restricted cloud tunnel and reconnection. Route the cloud to one selected business backend; never share or copy a live workspace database between two running services.
- Start with read-only printer status and print-start disabled. Physical controls and printing come last, with explicit authorization and onsite confirmation. Do not make Jetson and the existing NAS concurrent controllers of the same print job.

Real Jetson execution, performance, camera and printer results remain pending.
