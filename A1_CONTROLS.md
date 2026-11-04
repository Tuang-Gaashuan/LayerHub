# A1 control research

[简体中文](A1_CONTROLS.zh-CN.md) · October 29, 2026 · Bare A1, without AMS.

Research only: no NAS/printer connection, control request, runtime configuration change or deployment. Official source establishes candidate paths, not compatibility with the installed firmware. Existing hardware evidence is listed in [CONTROL_VALIDATION.md](CONTROL_VALIDATION.md).

| Area | Candidate path | Evidence / remaining work |
| --- | --- | --- |
| Light | `system/ledctrl`, `chamber_light` | Onsite on/off verified; show last accepted request |
| Temperatures | Legacy `gcode_line` temperature setters | Targets and return to zero verified; not accuracy |
| Part fan | Legacy P1 fan control | 20 percent/off verified; finer targets remain unvalidated |
| Pause/resume/stop/speed | Task and speed commands | Existing implementation; attended A1 print regression pending |
| Homing/XYZ | Capability-selected native or legacy path | Separate A1 adapter and physical validation required |
| Extrusion | Relative E movement or newer extrusion command | Not enabled; distinct from complete loading/unloading |
| External loading/unloading | Filament-change workflow | External-spool branch exists; actual A1 procedure pending |
| Material metadata | Filament settings | Does not establish measured spool grams |
| Calibration/object skipping | Capability-dependent commands | Research candidates only; not universally applicable |

Task, temperature and filament details: [official DeviceManager](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp). Dedicated implementations: [light](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevLampCtrl.cpp), [fan](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevFan.cpp). These source-level interfaces are not a promised stable third-party API.

## Motion findings

A1 is configured as i3. Official axis controls transform Y/Z differently from CoreXY. Following the source's UI logic, X preserves the input sign, Y reverses it, and the Z-up button produces a positive axis value. This is a **source-derived mapping**, not a verified physical direction on this printer. Label A1's moving objects separately: X toolhead, Y bed, Z gantry; validate directions onsite.

Native homing/jogging are selected by capability bits 32/38. Missing capability evidence must remain unknown. The legacy route bundles relative movement with reference/limit context; it must not be reduced to a bare move or used as an automatic fallback after rejection. The UI has 1/10 small/large steps, not a 10 mm travel limit.

Do not infer homing from zero/missing flags, elapsed time or command acceptance. Keep pre-send checks, a shared command lock, persistent request IDs, single-send delivery and explicit review after uncertain movement. A legacy adapter needs independently validated homing evidence and context handling before enabling it.

Sources: [A1 model configuration](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/resources/printers/N2S.json), [axis controls](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevAxisCtrl.cpp), [capabilities](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevAxis.cpp), [compatibility defaults](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevAxis.h).

## Filament and scope

Manual extrusion and complete loading/unloading are separate workflows. The official UI's actual-temperature gate is not a material-specific extrusion recommendation. Validate material temperature, modal side effects and physical completion independently. External-spool identifiers are protocol routes, not inventory IDs. Bare A1 has no extra chamber/auxiliary fans, dual nozzles or AMS dryer to expose. Generic commands do not establish such hardware support.

When attended testing resumes: verify homing, then individual positive/negative small steps; consider 10 mm only afterwards. Test filament operations separately. Pause/resume/speed belong to a normal authorized print; stop needs an interruptible test. No such tests occurred during this research. [Official UI and workflow](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/StatusPanel.cpp).

Research sources are pinned to `79df7ae309be398ed30c9f397c13c9911c2edc57`; private QA retains 17 official files and a SHA256 index. No private device identifiers are included here. Production control code is unchanged, and A1 motion remains disabled.

## Attended validation update — 2026-10-29 (paused)

A1 firmware 01.08.01.00 accepted legacy G-code homing and reported fresh XYZ homed flags (0 → 7). Homing and one positive X step were observed onsite as stopped and without issues, with the nozzle moving right for X. Y movement was not visually confirmed, despite success receipts for the first step and a separately authorized slow three-step request. The user requested stopping for later onsite inspection. Y remains unresolved and Z was not sent. No auto retry, printing, settlement, stock changes or production motion UI enablement occurred. Reverse directions, 10 mm moves and measured positional accuracy are not validated.
