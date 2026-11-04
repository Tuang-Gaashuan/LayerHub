# X2D AMS 进换退料 / Filament control

设备控制台 → 耗材与供料 → AMS 进换退料。首批开放 X2D 主喷嘴（逻辑编号 1）与 AMS 2 Pro 单元 0 的四个料槽，仅支持设备完整上报的 PETG。A1、其他 AMS 组合、外挂料及跨材质换料仍未开放。

选择颜色不会操作设备。先确认人在现场、打印板和供料路径安全，再选择进料/换入或退回当前耗材，核对目标料槽、颜色和设备温度中值后，点击确认执行一次。机器自动加热、切料与送丝。设备有确认出料提示时，应现场处理。没有远程自动补发完成、复位或异常恢复指令。

温度取自当前与目标料槽新鲜上报的喷嘴范围中值，范围必须有效且在 200–300°C 内。测试使用 PETG Basic 的 230–260°C 范围，目标 245°C；没有修改耗材参数或切片文件。

后台只发一次 QoS0 请求。成功回执不代表完成：还需新鲜料源、喷嘴有料传感器、非忙碌、步骤结束及无告警，稳定确认后才标记完成。X2D 正常装料结束可保持 AMS 辅助模式，不能要求所有状态整数归零。空喷嘴的 0xfeff 料源哨兵不代表真的装入外挂料。

装换退料与打印、其他控制共享同机互斥。打印/传输/未结算任务、连接过期、配置不匹配、告警或未核对指令均阻止执行。超时/断线保留待核对记录，不重发；目标料源或退料传感器未达到预期时，不能用核对按钮强行解除。后台重启也不恢复或重发这类动作。

不创建打印任务，不登记取件，不扣减库存克数；排料实际用量需要另行称量。实机验证为黄（槽1）→蓝（槽3）→白（槽4）→退料，现场逐步确认通过；不代表其他型号或所有 AMS 指令已经验证。

## English

Open Printer console → Materials and feed → AMS load, change and unload. This release enables only the X2D main extruder (logical ID 1), AMS 2 Pro unit 0, and PETG with complete device-reported slot metadata. A1, other AMS combinations, external spools and cross-material changes remain disabled.

Slot selection is inert. Confirm onsite clearance, choose load/change or unload, check the slot, actual colour and temperature midpoint, then confirm one operation. Heating, cutting and feeding are automatic. Handle any device-screen extrusion prompt onsite. No automatic done/reset/recovery commands are sent.

Each operation uses one QoS0 request and a shared per-printer command lock. An acceptance receipt is insufficient: fresh source, filament sensor, ended step, not-busy state and no alerts must independently confirm completion. Valid completed assist states are accepted; empty-source sentinels do not imply a loaded external spool. Unknown outcomes remain locked for review with no retry, including across service restart. Review cannot bypass a mismatched source or incomplete unload.

Operations require an idle printer, no active/unsettled task, fresh PETG metadata and valid reported temperature limits within 200–300°C. The attended PETG Basic test used a 230–260°C range, midpoint 245°C. No print, pickup, settlement or stock deduction is performed. The tested sequence was yellow slot 1 → blue slot 3 → white slot 4 → unload. This is not validation of other devices or every AMS command.
