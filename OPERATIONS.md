# Upgrades, backups and troubleshooting

[简体中文](03-升级备份与排障.md)

## Upgrade safely

1. Use a maintenance window with freshly idle devices, no active slicing/uploads/starts/uncertain commands and no pending restore.
2. Export a workspace backup as the owner. Save environment settings and the installed version separately. Backups contain private data and device secrets.
3. Stop the application, then back up the complete data directory and `/etc/layerhub`, preserving permissions and master.key.
4. Stage a new version directory, verify hashes and switch current to matching frontend/backend files. Keep the old version.
5. Check health, authentication, existing jobs/inventory/projects and device freshness. Preserve the print-start setting; do not create a physical print just to test deployment.
6. Never overwrite normal new business data merely to force a rollback. New formats require matching readers; preserve new data before explicitly restoring an older backup.

Restores clear sessions and disable devices. Do not run native and Docker instances against the same workspace. Copying only a live sqlite file can omit WAL data and encryption keys.

## Troubleshooting

| Symptom | Check first |
|---|---|
| Cloud 503 | NAS power/suspend/network, layerhub and tunnel services, then cloud loopback 15181 |
| Tunnel permission denied | Correct NAS public key, ownership/modes, effective Match settings, HostName/known_hosts match |
| Reverse-forward failure | Occupied old listener, PermitListen/key restrictions; never switch the listener to 0.0.0.0 |
| Login does not persist | Secure cookies combined with plain HTTP; use the correct public HTTPS origin |
| Printer offline | Client isolation/VLAN, DHCP changes, MQTT/FTPS/passive ports, access code and independently verified certificate pin |
| Read works, controls fail | Firmware authorization, LAN/Developer Mode and active-job ownership |
| Slicer unavailable | Pinned executable/profile paths, graphical runtime dependencies and permissions; core Docker image has no Studio runtime |
| Email absent | Optional worker enabled, matching private token, 163 authorization code, verified recipient and notification preferences |
| Empty workspace after restart | Wrong data directory or named volume; locate the original before initializing again |

Use `journalctl -u layerhub`, `journalctl -u layerhub-tunnel` and `nginx -t` locally. Redact logs before sharing. Reconnection is not confirmation that an uncertain print command executed; inspect job/device state before manual recovery.
