# Optional slicing engine / 可选切片引擎

The engine is **not bundled**. Reference validation used official Bambu Studio **v02.08.02.61**, Linux Ubuntu 24.04 AppImage and Windows CLI. Obtain the release independently from [Bambu Studio releases](https://github.com/bambulab/BambuStudio/releases/tag/v02.08.02.61), verify its source and retain its license notices. Do not install a networking plugin for the CLI slicing path. This package does not supply one.

本包不含引擎。参考版本是官方 Bambu Studio v02.08.02.61；请从官方发布页独立取得并核验。Linux 参考文件为 `BambuStudio_ubuntu24.04-v02.08.02.61-20260820225108.AppImage`，此前验收文件 SHA256：`d501b103fac5424513ec0e8d6bc145fb30719de2c7d94d7320d723740c81a7fd`。此值标识已验收文件，不能替代部署者对下载来源的核对。

## Native Linux example / 原生 Linux

1. Download the official AppImage as your ordinary user. Verify the file, then extract it **without root** in a disposable directory with `./BambuStudio.AppImage --appimage-extract`.
2. Install the extracted `squashfs-root` under `/opt/layerhub-slicer/runtime/` as root-owned files, after checking that no installation already exists. Do not replace a live engine during slicing.
3. Install the runtime's documented libraries. Typical Debian/Ubuntu requirements include Xvfb, xauth, Mesa GL, GTK3/WebKitGTK; names differ by distribution. Inspect `ldd` and the official release notes instead of assuming an AppImage is entirely self-contained. The already-tested NAS used a working extracted runtime; this package does not auto-install or verify every fresh OS library combination.
4. On a newly installed runtime:

```sh
sudo apt install -y xvfb xauth libgl1 libgtk-3-0 libwebkit2gtk-4.1-0
sudo python3 deploy/slicer/prepare-x2d.py \
  --runtime /opt/layerhub-slicer/runtime/squashfs-root \
  --profiles /opt/layerhub-slicer/profiles-x2d \
  --wrapper /opt/layerhub-slicer/run-studio
sudo -u layerhub /opt/layerhub-slicer/run-studio --help
```

The script recursively flattens only the named X2D/0.4/PETG profiles and records source hashes. It refuses existing outputs. The wrapper uses software GL, two Mesa threads, a 1GiB data-segment limit, 175 CPU seconds, low priority and a private Xvfb display with TCP disabled. The app's task timeout remains three minutes and slicing concurrency remains one.

脚本不接触业务数据库和打印机，也不执行切片。它生成的 wrapper 会限制资源。环境中需设置：

```text
LAYERHUB_SLICER_PATH=/opt/layerhub-slicer/run-studio
LAYERHUB_SLICER_PROFILES=/opt/layerhub-slicer/profiles-x2d
```

Restart the app only in an idle maintenance window. Verify a small synthetic STL, inspect dimensions, real paths and estimates, then verify object overrides separately. A successful slice is not permission to print and does not extend material/machine validation. Keep live print start off until attended validation.

Docker's supplied base image does not contain Studio/Xvfb/GUI libraries. For a containerized engine, build and test a separate, appropriately licensed runtime image and mount writable work data plus read-only runtime/profiles. Do not mount the production NAS data directory into a second container. Native deployment is the complete reference path in this alpha.

修改引擎或对外分发其二进制时，需保留相应许可证与对应源码义务。项目自身 AGPL 与第三方引擎声明见根目录 THIRD_PARTY_NOTICES.md。
