# 打印准备 / Print preparation

Beta 接受 Bambu Studio 等切片软件导出的已切片 3MF。检查完成后进入统一准备界面：左侧选盘与预览，右侧确认目标设备、实际料卷颜色与打印选项。每盘显示自己的时长、切片用量与整数克预留；同一料卷的多盘预留按合计校验。

单盘、设备空闲且没有前序任务时，确认打印板已清空后，可以一次提交完成入队、保存供料准备、传输和请求启动。也可以仅加入队列。多盘或设备忙碌时只入队，分别保存每盘的准备；不会自动开始下一盘。服务端在保存、传输和启动时保留原有权限、机型、库存、队首、遥测与供料检查。

快捷准备当前支持单耗材、单喷嘴盘。多耗材、多喷嘴、缺少时长或用量等情况，会指引使用原有高级准备流程；不会把其他机型、材质或未知工艺自动转换成可打印文件。

“取消本次上传”退出准备并返回设备队列。检查中会中断浏览器请求并忽略迟到响应。任务已提交后显示“返回任务列表”，保留恢复编号，返回页面不表示服务器任务已取消。等待/已传输且从未启动、无待核对结果的任务，可以取消；记录和文件保留，预留释放，不扣库存，也不开始下一项。实际打印中的停止仍通过设备控制台单独确认。

导入使用与原批量接口兼容的恢复编号。未知响应只允许读取原回执或显式使用原编号重试导入；不会因刷新、重新打开、读取回执或导入回放而重新启动。启动请求已发出时不能把关闭页面当作停止机器，必须核对真实状态。

# English

Beta accepts sliced 3MF files exported by Bambu Studio or another compatible slicer. After inspection, one preparation view combines plate previews, printer selection, visible spool colors, material reservations, and print options. Plate IDs, estimates, and material weights remain independent. Reservations from multiple plates sharing a spool are checked together.

For one plate on an idle printer without earlier queued tasks, explicitly confirming a clear bed allows one submission to create the task, save its material mapping, transfer the file, and request a start. Queue-only is also available. Multiple plates and busy printers only enqueue tasks; later plates never start automatically. Existing server checks still apply at every stage.

Quick preparation supports single-material, single-nozzle plates. Complex mappings or missing estimates are directed to the existing advanced workflow, without converting printer models, materials, or sliced processes.

Cancel this upload leaves preparation and returns to the device queue. Cancelling inspection aborts the browser request and ignores late results. After submission, Back to tasks preserves the recovery ID and does not cancel a server-side task. A never-started waiting or transferred task without unresolved execution results may be cancelled: its record and file remain, its reservation is released, and no inventory is deducted. Stopping a running printer requires a separate confirmation in the printer console.

Import recovery uses the existing idempotent batch API. Reloading, reopening, reading a receipt, or replaying an import never starts a print. If a start was already requested, leaving the page cannot be treated as proof that the machine stopped; check the real task and device status.
