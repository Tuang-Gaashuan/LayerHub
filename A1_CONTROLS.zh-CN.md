# A1 控制指令研究

[English](A1_CONTROLS.md) · 研究日期：2026-10-29 · 范围：A1 裸机、无 AMS。

本次只研究官方源码和本地记录，没有连接 NAS 或打印机、发送指令、修改运行配置或部署软件。源码中存在入口，不等于这台 A1 的固件已接受该指令。此前的灯光、温度目标和模型风扇实测证据见[控制验证记录](CONTROL_VALIDATION.md)。

## 结论与指令表

**A1 有继续扩展移动与维护控制的依据。当前障碍是缺少独立适配和实机验收，不能只把 X2D 的按钮放开。** 官方将 A1（N2S）登记为 i3 结构，轴控制按结构和能力分支处理。[型号配置](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/resources/printers/N2S.json) · [轴控制](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevAxisCtrl.cpp)。

| 操作 | 官方源码中的入口 / 参数要点 | LayerHub 与本机证据 |
| --- | --- | --- |
| 灯光开关 | `system/ledctrl`，节点 `chamber_light`、模式 on/off | 已实现，现场验证亮灭；界面显示上次接受的请求，不把静态灯光遥测当实际状态 |
| 喷嘴目标温度 | 旧路径经 `print/gcode_line` 承载 `M104`；新路径是另一能力分支 | 已实现旧路径，50°C 目标及归零反馈通过；未验证温控精度 |
| 热床目标温度 | 旧路径 `M140`；有对应能力时另有 `set_bed_temp` | 已实现旧路径，40°C 目标及归零反馈通过 |
| 模型风扇 | 旧路径 `M106`，模型风扇 P=1，S 为 0–255；新式风道有 `set_fan` | A1 旧路径的 20%及关闭通过；不能拿 X2D 风扇编号替代 |
| 暂停 / 继续 / 停止 | `print` 下的 `pause` / `resume` / `stop`；带独立序列号 | 已有通用实现，本轮 A1 打印中的完整回归未做 |
| 打印速度 | `print_speed`，param 为字符串档位 1–4 | 已有实现；不是 mm/s，A1 打印中的切换待实测 |
| 归位 | 原生能力存在时 `back_to_center`，否则官方空闲路径使用 `G28` | 当前只开放 X2D 原生路径；A1 未验证 |
| XYZ 点动 | 原生 `xyz_ctrl`；否则由 `gcode_line` 承载有上下文处理的相对移动 | A1 尚未适配；方向和归位证据需独立验证 |
| 少量挤出 / 回抽 | 旧路径涉及 `M83` 和 E 轴移动；新式挤出另有 `set_extrusion_length` | 尚未开放；不等同于完整装料 / 退料 |
| 外置耗材装料 / 退料 | `ams_change_filament`；官方界面也将虚拟外置料架送入此流程 | 指令名字含 AMS 不代表只能接 AMS；裸机 A1 仍需验证实际流程与用户配合 |
| 外置耗材信息 | `ams_filament_setting`，包括材质、颜色和温度信息 | 可作为后续研究；不能据此推断料卷实际克数，也不能自动扣库存 |
| 校准 | `calibration` 的 option 位选择项目 | 只记录候选；不是所有型号都支持所有位，不能全选统一发送 |
| 跳过对象 | `skip_objects`，对象列表 `obj_list` | 需切片对象标记、当前任务匹配和设备能力；不等于取消某盘 |

表中通用控制、温度、耗材流程、速度及校准取自[官方 DeviceManager.cpp](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp)；灯光见[DevLampCtrl.cpp](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevLampCtrl.cpp)，风扇见[DevFan.cpp](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevFan.cpp)。以上是源码研究结果，不是厂商承诺的稳定第三方 API。

## 移动适配的关键差异

**能力与协议选择。** 官方从 `fun` 的第 32、38 位分别读原生归位、原生点动能力；`home_flag` 低三位对应 XYZ。已有 A1 记录没有新鲜原生能力证据，不能把布尔值 false 当成已确认不支持，也不能把在线当成已归位。官方存在旧路径，为适配提供依据；是否适用于本机固件仍待验证。[能力解析](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevAxis.cpp)。

**方向。** 官方对非 CoreXY 的 Y/Z 变换符号。沿官方界面逻辑，X 输入保留符号，Y 输入反号；Z“上”按钮最终生成正向轴值，“下”生成负向轴值。这是对源码组合的推导，**不是这台机器已验证的物理方向**。A1 面板应分别标识喷头 X、打印板 Y、横梁 Z；Y 的前后标签需现场确认。

**步长。** 官方界面有 1 和 10 两档；原生 mode 分别为 0、1，旧路径则使用对应的相对位移。10 mm 是此界面的较大点动档位，不是机器最大运动行程。后续建议先验收单轴小步，再验证反方向和 10 mm。

**参考模式和限位。** 官方旧路径将 `M211`、`M1002`、相对模式和移动组合处理；不能只发送一条位移，也不能简单在末尾强制绝对模式代替恢复原上下文。专有上下文指令的设备行为还需核实。源码中的 UI 检查顺序不照搬：LayerHub 所有检查应在发送前完成。

**完成证据。** 官方头文件有将零回零标记视为兼容状态的逻辑；LayerHub 不采用这个默认值。没有有效回零证据时，不能因为收到成功回执或等够时间就解锁点动。若旧固件无法提供可靠回零证据，需要明确设计现场归位核对流程；目前仍禁用。

**单次执行。** 后续应继续使用白名单、唯一请求号、同机互斥、一次发送和不确定结果锁定。旧路径不是原生指令被拒绝后的自动备用路线。断线、超时或重连不得重发相对移动。

依据：[轴控制实现](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevAxisCtrl.cpp)、[回零兼容逻辑](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevAxis.h)、[官方按钮与状态判断](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/StatusPanel.cpp)。

## 进退料与不适用功能

少量 E 轴挤出需区分目标温度和实际温度。官方界面以实际温度至少 170°C 作为手动挤出的门槛；这**不是 PETG 等材料的推荐挤出温度**。后续仍应采用实际材质温度、读数新鲜度和现场观察。旧式挤出涉及挤出模式，不能默认当前模式不受影响。

完整装料流程的旧式外置入口使用虚拟料架 254，退料目标为 255；新式流程还区分料架与挤出机。不能把这些数字当成库存编号，也不能把“挤出 10 mm”当成“已装料”。加热、切料、退回位置和用户插拔等完成证据均待本机验收。[官方流程入口与实际温度门槛](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/StatusPanel.cpp)。

裸机 A1 不展示额外腔体风扇、辅助风扇、双喷嘴或 AMS 烘干。官方 A1 配置也明确没有辅助／腔体风扇；即使未来加装 AMS lite，也不能据通用烘干命令推断具备烘干硬件。校准应按 A1 能力逐项筛选，不能套用其他型号的激光雷达或喷嘴切换项目。

## 后续验证顺序（本次未执行）

1. 先离线完成 A1 专用协议、方向、状态与权限检查；与 X2D 适配隔离。
2. 人在现场时确认空板、停稳及无待结算任务，先单独验证归位，再逐轴正反方向小步。
3. 小步通过后再考虑 10 mm；进退料单独安排，有实际耗材和现场配合。
4. 暂停、继续、速度在下次用户授权的正常打印中验证；停止留给可中断测试件。

源码固定于 `79df7ae309be398ed30c9f397c13c9911c2edc57`。本地研究目录保存 17 份官方文件、来源索引与 SHA256；未把它们复制进公开项目。本文不含设备地址、序列号、访问码或邮件资料。**没有改动任何生产控制实现，A1 移动保持未开放。**

## A1 移动补充实测（2026-10-29，已按用户要求暂停）

A1 裸机、固件 01.08.01.00，使用官方旧式 G-code 路径的私有受限程序。归位收到 success，新鲜 XYZ 回零标记由 0 变为 7，用户确认停稳无异常；X 正向 1 mm 指令收到 success，现场看到喷头向右小幅移动并停稳。未测量精确距离。

Y 首次 1 mm 后用户未看清位移，确认停稳后明确要求增加测试；另发一个包含三段慢速 1 mm 的独立请求。设备回复 success，但用户仍未观察到 Y 移动，随后要求暂停、稍后现场检查。**Y 保留待核对，不能标为通过；Z 未发送。** 没有自动重发。末次状态在线空闲、无告警，未改变任务和库存，正式 A1 移动按钮仍未开放。

Attended A1 legacy-motion testing passed homing (fresh XYZ flags plus onsite confirmation) and one positive X step (rightward motion seen onsite). Y receipts succeeded, but movement was not visually confirmed, including the separately authorized three slow 1 mm segments. Testing was paused at the user's request with Y unresolved; Z was never sent. Direction reversal, 10 mm steps and positioning accuracy remain untested. The production A1 motion UI remains disabled.
