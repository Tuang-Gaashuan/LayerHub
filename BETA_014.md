# v0.1.4-beta: spool history, print completion and recovery

[简体中文](BETA_014.zh-CN.md)

Slice in Bambu Studio, export a sliced `.gcode.3mf`, then use LayerHub to manage team printers and queues. The model workbench remains outside the Beta workflow.

## Spool activity

Open **Activity history** in the material library to review registration, mount/unmount, details and stock adjustments, normal job settlement, and usage reconciliation for missing jobs. Entries show the actor, time, job identifier, before/after balance and feed position. Expand a details change to inspect edited values.

History is committed in the same database transaction as inventory changes. A logging failure rolls back the stock update. Device percentage estimates do not overwrite inventory grams. Collection and confirmed usage remain necessary for deduction.

Existing spools receive an **Inventory at activation** entry during upgrade. Earlier actions are not reconstructed. Current spool visibility rules apply, including private inventory. Deletion entries are retained in the database; this version does not provide a separate browser for deleted spools.

When loading a previously registered physical roll, select its existing record. Identity, color, balance and history are preserved. Register a new spool only for a previously unregistered physical roll. Differences in reported device color produce no warning or mount restriction and never overwrite the library color. Material forms return to their original printer tab and scroll position.

## Completion entry

**Print completion** lists ended/stopped jobs the viewer can manage that still need settlement. The dashboard, queues and personal task list show a pending count.

Device completion is machine evidence; collection and a clear bed require human confirmation; settlement records usage in inventory. Opening this entry never collects a print, deducts material or starts the next plate. Stopped jobs require measured consumption; normally completed jobs can use explicitly labelled slicer estimates or measured usage.

## Recovering a lost response

Material registration, edits, replacement, deletion and normal job settlement carry a one-time operation identifier. A server receipt is committed with the mutation. After a lost response, the browser reconnects and queries that identifier using read-only requests. A committed receipt confirms the save and refreshes the display. An open error form receives a **View latest status** action.

An absent receipt remains **unknown**, not proof that the operation failed. LayerHub never automatically resends the mutation or retries filament feeding, motion or print-start commands. The same identifier cannot execute twice. Pending identifiers remain scoped to the account and browser session, and do not store form contents, passwords or printer access codes.

Printer commands retain their separate device-receipt and manual-review rules. A server receipt does not prove that a physical device performed an action.

## Installation, upgrade and scope

Use the package matching the host CPU architecture. Follow the existing cloud—NAS—printer deployment guide for first installation and back up the database and encryption key before upgrading. Windows and Linux AMD64 packages share the same version and corresponding source.

History and receipts use additive tables. Existing usage records are not rewritten. Keep the upgraded program with its data; alternating older programs against the same workspace creates activity gaps. For a rollback, stop the service and follow the backup recovery guide rather than replacing a running database.

ARM64 remains preliminary. A previous build was exercised on Jetson Orin Nano 8GB / Ubuntu 22.04 for basic deployment. Cross-building a new revision does not revalidate it on that device, nor establish printer, camera or long-term production acceptance.

If a receipt remains unknown, inspect current spool/job records and explicitly dismiss the browser notice. This acknowledgement neither changes inventory nor clears device-control review locks.
