# 控制验证与待发布修复 / Controls and pending fix

2026-10-29，设备由现场人员确认清空并配合观察。实机记录为当前部署版本的结果；本页的风扇档位修复仍是本地待发布修改。私人设备信息和原始记录不公开。

| 设备 | 本轮通过的检查 | 边界 |
| --- | --- | --- |
| X2D | 灯光；两个喷嘴目标 50°C、热床目标 40°C及归零；四路风扇 20%及关闭；归位与 XYZ 各一次正向小步 | 用户逐轴确认有移动、停稳、无异常；未测定位移精度 |
| A1 | 灯光亮灭由现场确认；喷嘴目标 50°C、热床目标 40°C及归零；模型风扇 20%及关闭 | 灯光显示上次接受的请求；未验证 XYZ |

温度检查验证目标设置与反馈，不代表稳态温控精度。结束时设备空闲、无告警或未核对指令，加热目标及测试风扇均为零，灯光恢复原设置。没有新建打印、取件结算或库存扣减。未验证打印中的暂停、继续、停止、速度切换，基础控制轮未验证进退料、10 mm、喷嘴切换或 AMS 烘干；当日晚间的 X2D AMS 补充实测见下节。

## 风扇档位修复（待发布）

实机请求 X2D 模型风扇 25%，得到新鲜回报 20%，原有匹配逻辑因此超时待核对。现场确认转动正常后，人工核对并关闭；没有重复发送该指令。

本地修复采用保守的整十档输入：滑杆与数字输入步长为 10%，快捷值为关闭、20%、50%、80%、100%。非整档输入直接拒绝，不能变成后台待确认指令；旧回报中的任意百分比仍按原值显示。打开编辑器只生成本地草稿，选值不会操作设备。

保留新鲜度检查和原有反馈精度范围；没有把现代风扇的成功容差放宽至 10%。关闭需精确回报 0，非零请求不能由 0 确认。20% 与关闭已有实机证据；其余整十档只做了离线验证，不能宣称全部实测通过。

## A1 移动研究

官方源码按能力位选择原生控制或旧式 G-code，并按机器结构处理 Y/Z 方向。A1 先前没有上报原生能力，这不代表它绝对无法移动；但当前 LayerHub 只适配了 X2D 原生路径。[官方轴控制](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevAxisCtrl.cpp) · [官方能力解析](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevAxis.cpp)。

后续 A1 适配需独立实现方向、回零证据、参考模式和软限位上下文处理，以及单次发送和不确定结果核对。首先离线验证，再在有人在场时单轴小步验收。当前不暴露 A1 移动按钮、不自动回退协议，也不向设备探发指令。

## English

On October 29, attended checks covered X2D lights, both nozzle targets, bed target, four fans at 20%/off, homing and one positive small-step request on each axis. Each movement was confirmed onsite as visible, stopped and without issues; distance was not measured. A1 checks covered onsite light on/off, nozzle/bed targets and part fan at 20%/off. Temperature targets do not establish temperature accuracy. Heating targets/fans returned to zero; lights returned to baseline. No printing, pickup, settlement or stock mutation occurred.

The **local, unreleased fan fix** uses ten-percent selectable levels and rejects unsupported inputs before command creation. It prevents the observed 25%-requested/20%-reported mismatch without loosening modern feedback tolerance. Fresh zero feedback is mandatory for off; zero cannot confirm start. Only 20%/off have hardware evidence; other levels remain offline-tested. The UI keeps raw reported percentages visible and never applies an editor draft automatically.

A1 motion remains unavailable. Official source has native and legacy paths with architecture-dependent directions; missing native capability is not proof that all motion is impossible. A separate legacy adapter, state/limit checks and attended directional validation are required. No motion probe or protocol fallback was performed. Print pause/resume/stop/speed, extrusion, 10 mm moves and additional AMS functions were not tested in this round.

## X2D AMS 进换退料补充实测（2026-10-29）

在用户现场确认下，通过 AMS 2 Pro、主挤出机编号 1，完成 PETG Basic 黄色 1 号槽进料 → 蓝色 3 号槽换料 → 白色 4 号槽换料 → 白色退回的完整流程。采用设备登记的 230–260°C 中值 245°C。每步仅发一次 `print/ams_change_filament`，并同时核对新鲜来源/传感器/忙碌状态与现场出料、退料结果；无告警，最终目标温度归零。没有新打印或耗材结算。

这是 X2D + AMS 2 Pro 的有限实测，不覆盖 A1、其他供料器、裸机外置料架、远程完成确认 `ams_control done` 或其他 AMS 指令。正式界面尚未新增供料按钮。

The attended X2D + AMS 2 Pro test passed yellow load → blue change → white change → unload using PETG Basic at a configured 245°C. Every request was sent once and checked against fresh telemetry and onsite observation. All heaters returned to zero targets. This does not validate A1 filament workflows, external-spool loading, `ams_control done`, other AMS hardware or a production filament-control UI.

## A1 移动补充实测（2026-10-29，已按用户要求暂停）

A1 裸机、固件 01.08.01.00，使用官方旧式 G-code 路径的私有受限程序。归位收到 success，新鲜 XYZ 回零标记由 0 变为 7，用户确认停稳无异常；X 正向 1 mm 指令收到 success，现场看到喷头向右小幅移动并停稳。未测量精确距离。

Y 首次 1 mm 后用户未看清位移，确认停稳后明确要求增加测试；另发一个包含三段慢速 1 mm 的独立请求。设备回复 success，但用户仍未观察到 Y 移动，随后要求暂停、稍后现场检查。**Y 保留待核对，不能标为通过；Z 未发送。** 没有自动重发。末次状态在线空闲、无告警，未改变任务和库存，正式 A1 移动按钮仍未开放。

Attended A1 legacy-motion testing passed homing (fresh XYZ flags plus onsite confirmation) and one positive X step (rightward motion seen onsite). Y receipts succeeded, but movement was not visually confirmed, including the separately authorized three slow 1 mm segments. Testing was paused at the user's request with Y unresolved; Z was never sent. Direction reversal, 10 mm steps and positioning accuracy remain untested. The production A1 motion UI remains disabled.

## A1 最新补充结论（2026-10-29） / Latest A1 follow-up

本节更新前述暂停时的历史结论。用户后续明确确认 Y 轴发生移动，方向未看清，原待核对记录已完成人工核对。随后重新授权并完成归位，再执行一次正向 Z 轴 1 mm 点动；设备回执成功，用户确认喷头横梁小幅上升、停稳无异常。新归位和 Z 记录均已核对，未自动重发。

A1 归位与 XYZ 均已有现场移动观察；X 向右、Z 上升，Y 方向仍未确认。精确距离、反方向、10 mm 与极限行程未验证。测试由私有受限程序完成，正式 A1 移动面板仍待适配，不能视为完整控制功能已经上线。

This supersedes the earlier paused-test status: the user subsequently confirmed Y movement, with direction unknown. A newly authorized home was followed by one positive 1 mm Z request; the device acknowledged success and the onsite user confirmed upward crossbeam motion, stopped without issues. Both new audit records were reviewed, with no automatic resend. Exact displacement, reverse motion, 10 mm moves and travel limits remain untested. These attended private checks do not enable the production A1 motion UI.
