# 后台文件 / Background files

文件上传完成后，服务端可继续排队解析。页面离开或重新登录后，在上传区域的“后台文件”里继续安排；仅显示当前账号的文件，解析不会创建打印任务或启动机器。每账号保留最多 8 份草稿，可手动删除已完成或失败的草稿。恢复后使用服务器保存的原文件，不需要重复上传。

关闭浏览器时若上传尚未完成，仍可能中断；本功能不等于分片断点续传。此功能已在 2026-10-31 部署至当前云端与 NAS。

After upload completes, server-side parsing can continue while the page is closed. Return to Background files in the upload area to resume setup. Files are private to their account; parsing does not create print jobs or start printers. Each account can retain up to eight drafts and remove completed or failed drafts. Resume uses the retained server file without another upload. Closing the browser before upload completion can still interrupt the upload; chunked resumable upload is not provided. This feature was deployed to the current cloud and NAS on 2026-10-31.


## 自动衔接打印确认 / Automatic continuation

上传完成后，文件仍由 NAS 后台解析。保持等待窗口时，解析完成会自动进入设备、打印盘与耗材确认，不需要再点击“继续安排”。选择“后台处理，稍后安排”、离开页面或关闭浏览器后，解析继续，回来可从后台文件恢复。上传本身仍需页面保持打开；这不是断点续传。解析完成不自动创建任务或启动打印。

After upload, the NAS parses the file independently. Keep the waiting dialog open to automatically enter device, plate and material confirmation when ready. Choose Continue in background, leave the page, or close the browser to resume later from the background file list. Upload itself still requires the page to stay open; resumable upload is not provided. Parsing never creates a job or starts a print automatically.
