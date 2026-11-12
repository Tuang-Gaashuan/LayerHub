# Pause offline notifications during maintenance

As an administrator, open **System diagnostics → Maintenance mode**, choose 30 minutes, 1 hour, 2 hours or 4 hours, and select **Pause offline notifications**. Wait for gateway confirmation before disconnecting a printer, the NAS or the server for maintenance.

This workspace-wide setting pauses printer offline and NAS connection offline notifications. Connection status, diagnostics and logs remain available. Print completion, device faults and other notifications are unaffected. Messages already accepted by the mail server cannot be recalled.

Notifications resume automatically at expiry. You can also end maintenance early; wait for gateway confirmation of that change. A continuing outage is subject to a new three-minute delay after maintenance, rather than replaying maintenance outages.

Both the NAS backend and gateway mail worker must be updated. The worker persists the window independently before acknowledging it, so a disconnected NAS or restarted worker does not lose a confirmed maintenance window. A locally saved setting without gateway confirmation does not guarantee that NAS offline notifications are paused. Changes are audited; restoring a backup does not reactivate an old maintenance window.

This feature does not control printers, collect prints, settle material use or disable device safeguards.
