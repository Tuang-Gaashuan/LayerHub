# Third-party notices / 第三方声明

LayerHub original code and original schematic artwork: AGPL-3.0-only. Third-party components retain their licenses.

## Go dependencies

Developer-only npm dependencies (jsdom, @babel/parser and parse5) are recorded in source/qa/frontend-deps/package-lock.json. Their installed packages are not bundled in runtime archives; their respective upstream licenses apply. Locale catalogs and compiled text patches are original project data.

The matching dependency source is included in source/vendor/. License copies are retained both there and in licenses/go/.

| Module | Version | License files |
|---|---|---|
| `github.com/bluenviron/gortsplib/v5` | `v5.6.7` | LICENSE |
| `github.com/bluenviron/mediacommon/v2` | `v2.9.6` | LICENSE |
| `github.com/dustin/go-humanize` | `v1.0.1` | LICENSE |
| `github.com/eclipse/paho.mqtt.golang` | `v1.5.1` | LICENSE, NOTICE.md |
| `github.com/google/uuid` | `v1.6.0` | LICENSE |
| `github.com/gorilla/websocket` | `v1.5.3` | LICENSE |
| `github.com/jlaffaye/ftp` | `v0.2.4` | LICENSE |
| `github.com/mattn/go-isatty` | `v0.0.24` | LICENSE |
| `github.com/ncruces/go-strftime` | `v1.0.0` | LICENSE |
| `github.com/pion/logging` | `v0.2.4` | LICENSE |
| `github.com/pion/randutil` | `v0.1.0` | LICENSE |
| `github.com/pion/rtcp` | `v1.2.19` | LICENSE |
| `github.com/pion/rtp` | `v1.10.5` | LICENSE |
| `github.com/pion/sdp/v3` | `v3.0.20` | LICENSE |
| `github.com/pion/srtp/v3` | `v3.1.3` | LICENSE |
| `github.com/pion/transport/v5` | `v5.1.1` | LICENSE |
| `github.com/remyoudompheng/bigfft` | `v0.0.0-20230129092748-24d4a6f8daec` | LICENSE |
| `golang.org/x/net` | `v0.59.0` | LICENSE |
| `golang.org/x/sync` | `v0.23.0` | LICENSE |
| `golang.org/x/sys` | `v0.48.0` | LICENSE |
| `modernc.org/libc` | `v1.77.1` | LICENSE, LICENSE-3RD-PARTY.md |
| `modernc.org/mathutil` | `v1.7.1` | LICENSE |
| `modernc.org/memory` | `v1.12.1` | LICENSE, LICENSE-GO, LICENSE-LOGO, LICENSE-MMAP-GO |
| `modernc.org/sqlite` | `v1.60.1` | LICENSE, LICENSE-3RD-PARTY.md, LICENSE-SQLITE, LICENSE-SQLITE_VEC |

## Web runtime

- Three.js 0.180.0 and its OrbitControls / STLLoader / TransformControls: MIT; see source/web/vendor/three/LICENSE and source.json. Imports in addons point to the bundled modules.

## Fonts

The following unmodified font files retain SIL OFL-1.1 notices under source/web/assets/fonts/:

- @fontsource-variable/noto-sans-sc 5.3.1 — OFL-1.1; LICENSE and source integrity metadata included.
- @fontsource-variable/noto-serif-sc 5.3.1 — OFL-1.1; LICENSE and source integrity metadata included.
- @fontsource/lxgw-wenkai 5.3.0 — OFL-1.1; LICENSE and source integrity metadata included.
- @fontsource/zcool-kuaile 5.3.0 — OFL-1.1; LICENSE and source integrity metadata included.
- @fontsource/zcool-xiaowei 5.3.0 — OFL-1.1; LICENSE and source integrity metadata included.

## Slicer-derived data and optional engine

The embedded PLA Basic / PETG HF configuration data are flattened/adapted from Bambu Studio profiles. The original upstream license is AGPL-3.0; upstream: https://github.com/bambulab/BambuStudio . Tested reference release: v02.08.02.61. Derived configuration changes do not modify the upstream engine. This distribution includes no Bambu Studio executable, AppImage, networking plugin or downloaded engine runtime. The optional engine must be acquired independently, retaining its own source/license obligations.

## Artwork and product names

Printer illustrations are project-created schematic SVGs with raster renders; generator sources are included in source/assets-source. They are not official product photographs or CAD. Product names and trademarks belong to their respective owners; no manufacturer endorsement is claimed.

## Development/build tools

Go compiler, Node.js, npm, Python, optional sharp and jsdom are development tools. jsdom/sharp runtime dependencies are not bundled into the deployed web app; pinned package manifests are supplied for test/asset reproduction. Docker base distribution packages, FFmpeg and other system software retain their own licenses in their respective distributions.

No private production database, printer model upload, access code, email account, SSH identity or mail relay token forms part of this source distribution.
