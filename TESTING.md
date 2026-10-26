# External acceptance and feedback

[简体中文](02-外部验收与反馈.md)

Use a fresh, independent test workspace first. Record package version, OS/CPU architecture, native/Docker mode, browser and—when relevant—printer model, firmware, supply modules, LAN Only and Developer Mode status. Do not post serial numbers, access codes, private emails, cookies, keys, tokens, backups or personal model files.

| Order | Test | Pass condition |
|---|---|---|
| 1 | Fresh startup | Unique random owner password; offline mode; print start off; healthy HTTP endpoint |
| 2 | Stop/restart | Existing accounts/projects persist; no password reset or automatic print |
| 3 | Two-member isolation | Private models/drafts remain private; admins cannot manage owners or peers |
| 4 | Sessions | Revoking one other personal login preserves the current session and other users |
| 5 | Shared projects | Explicit publication; independent copy; editing copies leaves originals intact; withdrawal blocks new copies |
| 6 | Projects/drafts | Dimensions, placement, object settings and empty boards survive reopening |
| 7 | Backup/restore | Owner-only export; restore into a separate offline test directory; accounts/projects retained, devices disabled |
| 8 | Cloud/NAS link | Loopback reverse listener only; public HTTPS; NAS loss yields 503 with no automatic write retry |
| 9 | Slicing | Real paths and estimates, preserved geometry/settings—not just a success label |
| 10 | Printer read-only | Fresh telemetry, temperatures, state and slots; loss of connection is visible |
| 11 | Attended controls | Confirm physical low-impact actions and fresh telemetry; authorization failures/uncertain commands handled correctly |
| 12 | Attended print last | Clear plate and validate material mapping; one start; start/finish notices belong to the job owner |

Without hardware, finish 1–9 and mark 10–12 untested. Without the optional email worker, email delivery remains untested. Device completion, physical collection and actual material settlement are separate events.

Suggested report: version/environment, model/firmware if relevant, reproduction steps, expected/actual behavior, impact on existing jobs, and redacted error codes/log excerpts. Sensitive reports belong in a private channel with the maintainer; never upload a whole data directory.
