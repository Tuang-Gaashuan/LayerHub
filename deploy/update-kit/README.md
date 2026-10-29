# LayerHub Download / Update Kit · v0.1.2-beta

[简体中文及完整操作说明](README.zh-CN.md)

GitHub public API requests are rate limited, including requests sharing the same proxy exit. When exhausted, the tool reports a retry time when supplied by GitHub. Retry later or transfer NAS packages from a trusted download computer and use the offline entrypoints. A failed network check is never reported as “already up to date.”

This kit is distributed alongside LayerHub v0.1.2-beta. Beta is the default and is recommended alongside Bambu Studio. The Windows downloader is self-contained. Linux updates target existing native Debian/Ubuntu amd64 systemd installations and need Python 3 for SQLite guards/backups. Fresh cloud setup, Docker, ARM and automatic cross-channel migration are outside this kit.

## Entrypoints

| File | Action |
|---|---|
| `Download-Beta-Windows.cmd` | Download, verify and extract the current Beta Windows application; never overwrite an existing folder or run the downloaded app |
| `Download-Beta-NAS.cmd` | Download the Beta Linux archive and verification receipt for transfer to NAS/cloud |
| `NAS-Update.sh` | Check and update this existing NAS installation after explicit local confirmation |
| `Cloud-Update.sh` | Update this existing cloud gateway UI after the NAS has the exact target version |
| `NAS-Update-Offline.sh` / `Cloud-Update-Offline.sh` | Use Linux archives plus `.verified.json` receipts in this folder's `downloads` directory |
| `Start.cmd` | Windows interactive menu |

Windows uses proxy environment variables or the system proxy without changing settings. Downloads use HTTPS, release checksums and GitHub asset digests when supplied. Interrupted transfers retain `.part` files for retry/resume. Existing extracted folders are kept intact.

On Linux, run `sudo sh NAS-Update.sh`, then `sudo sh Cloud-Update.sh` on the corresponding hosts. The kit never configures a new SSH connection or sends printer commands. A current-version deployment is a no-op. New versions require owner authentication if existing bootstrap credentials are no longer valid; passwords are hidden and never stored.

NAS updates require fresh idle/finished telemetry, no transfer/start/unconfirmed commands or restore/slicing activity, and a final `yes`. The app is stopped, a consistent SQLite snapshot and complete private data/config backup are saved under `/var/backups/layerhub-update/`, and matching new binary/web files are selected together. Environment settings, printer secrets, tunnel and SMTP configuration remain untouched. Old releases remain available.

Failures before starting the new app can return to the previous binary. Once the new reader starts it may migrate/write data: the kit never overwrites that data or blindly picks an older reader. It retains the matching program and backup for review. Cloud UI failures can restore the old gateway config, while NAS data remains untouched.

Offline receipts provide integrity, not digital signatures. Transfer both package and receipt from a trusted downloader via a trusted path. Offline mode does not discover newer remote releases.

Run `bin/layerhub-update-linux-amd64 nas --help` or `cloud --help` for path overrides. Omit `--apply` for preflight only. `--channel alpha` is explicit; updates must stay on the installed channel. The application packages still contain the first-install guide and script; this kit does not replace initial network, certificate or tunnel setup.

Sources and actual Go dependency code are included under `source/`; build with Go 1.27+ and `go build -mod=vendor -trimpath -buildvcs=false ./cmd/updater`. Original code is AGPL-3.0-only; third parties retain their licenses.
