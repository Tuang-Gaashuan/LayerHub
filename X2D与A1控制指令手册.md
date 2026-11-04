# X2D 与 A1 控制指令手册

**文档版本：1.0 · 整理日期：2026-10-29 · 用途：LayerHub 后续开发、适配和验收参考。**

本文件汇总当前可追溯的控制资料，正文说明常用参数，附录逐项列出命令及源码位置。只阅读这一份文件即可了解机型差异、项目支持状态和验证边界，无需翻阅聊天记录。

范围为官方 Bambu Studio 公开源码、X2D / A1 机型配置、LayerHub 当前实现及已保存的实机记录。**“全部”指本次资料范围内的完整归档，不代表掌握厂商未公开的全部固件指令。** 通用源码中的命令未必适用于这两款机器；附录保留不适用项以避免误用。源码固定至提交 `79df7ae309be398ed30c9f397c13c9911c2edc57`。

本次整理没有连接 NAS / 打印机、发送控制、重启服务或部署更新。文档不包含实际 IP、访问码、序列号、账号凭据或收件地址。出现的参数是协议说明，不是本次执行记录。

## 目录

1. [读法与支持状态](#readme)
2. [机型、编号与能力](#models)
3. [通信结构](#transport)
4. [文件、选盘和打印控制](#printing)
5. [温度、灯光、风扇和风道](#settings)
6. [归位、XYZ 与挤出](#motion)
7. [外置耗材与 AMS](#filament)
8. [校准与打印选项](#calibration)
9. [相机、录像和照片](#camera)
10. [告警、诊断和升级](#maintenance)
11. [LayerHub 接口与结果确认](#layerhub)
12. [实机验证记录与后续验收](#validation)
13. [完整命令索引](#index)
14. [来源与维护方法](#sources)

<a id="readme"></a>
## 1. 读法与支持状态

| 标记 | 含义 |
| --- | --- |
| 已实现 | LayerHub 有对应代码，不等于所有机型、所有固件均已实测 |
| 已实测 | 仅本文件明确列出的机器、参数和现场观察已通过 |
| 候选 | 官方源码存在入口，尚未在本项目开放或完成对应机型验收 |
| 条件适用 | 必须有相应硬件、能力及固件证据，不能仅凭品牌或型号发送 |
| 不适用 | 当前两台设备配置没有对应硬件，不能作为待测按钮直接开放 |

每条命令应同时核对：**外层分区 + 命令名 + 参数分支 + 机型/附件能力 + 结果证据**。例如 `print/stop` 停止打印，`pushing/stop` 停止状态推送，二者不是同一个动作。MQTT 发布成功、设备接受、实际状态变化和物理动作完成也不是同一件事。

<a id="models"></a>
## 2. 机型、编号与能力

| 项目 | X2D | A1（本项目为裸机、无 AMS） |
| --- | --- | --- |
| LayerHub 型号 ID | `x2d` | `a1` |
| 官方配置 ID | `N6` | `N2S` |
| 官方结构配置 | `core_xy` | `i3` |
| 移动对象 | 喷头 X/Y，热床 Z | 喷头 X，打印板 Y，横梁 Z |
| 喷嘴 | 两个独立逻辑编号 | 单喷嘴 |
| 模型风扇 | 新式风道编号 | 旧式 P1 风扇 |
| 其他风扇 | 按实时风道模式开放 | 不提供辅助 / 腔体风扇 |
| 腔温 | 官方配置存在可编辑能力；本项目未适配 | 不适用 |
| 本地视频路径 | 当前适配使用设备上报的 RTSPS | 当前适配使用 JPEG 通道，帧率受设备源限制 |
| 手动移动 | LayerHub 已开放原生归位和单次小步 | 尚未开放；旧式路径待独立适配 |

来源：[X2D 配置][S-N6]、[A1 配置][S-N2S]；实际硬件动作和视频接入状态以第 12 节及本机最新证据为准。

**X2D 喷嘴编号。** 固定源码配置把逻辑编号 0 标为辅助喷嘴、1 标为主喷嘴。它不是库存号，也不能照搬其他双喷嘴机型的左右含义。现有界面保留“喷嘴 0 / 1”，温度指令必须明确指定 `extruder_index`；供料和打印时还须核对实时绑定。

**X2D 外置料号。** 当前 LayerHub 将 `vir_slot` 255 映射为 `external`、254 映射为 `external:1`。这是项目槽位映射，不能直接据此断言物理左右或喷嘴绑定。旧式外置装料的目标值 254、退料目标值 255 则属于另一协议语境，不能混用。

**能力字段。** 官方 `fun` 的 bit 32 / 38 分别用于原生归位 / 轴控制；`home_flag` 低三位为 X/Y/Z 回零信息。X2D 新式热床设置还要核对对应能力（现有项目检查 bit 39）。能力字段缺失、过期和明确不支持要分开表示，不能用一个布尔 false 代替所有情况。[能力解析][S-DevAxis]。

**范围区别。** 官方 X2D 配置列出喷嘴 0–300°C、热床 0–120°C、腔温 0–65°C；A1 为喷嘴 0–300°C、热床上限 100°C。LayerHub 当前 X2D 热床采用更保守的 0–100°C，且没有腔温控件。这些是源码/项目限值，不能当成本机所有温度已验收通过；也不能用型号静态上限绕过实时能力。

<a id="transport"></a>
## 3. 通信结构

LayerHub 当前由 NAS 访问打印机，浏览器经云端操作 NAS。电脑系统代理不承担打印机局域网发现。正常网络通道与打印机控制权限需要分别确认；已能读取状态不等于所有写入已获固件许可。

| 通道 | 当前项目用途 | 备注 |
| --- | --- | --- |
| MQTT TLS，默认 8883 | 设备状态与控制 | 当前局域网用户名为 `bblp`，凭据来自设备访问码；不把真实值放进文档或日志 |
| `device/<serial>/request` | 发送设备请求 | serial 是占位符，不是设备 ID 或型号 |
| `device/<serial>/report` | 接收状态 / 回执 | 区分命令回显和独立遥测更新 |
| 隐式 FTPS，默认 990 | 上传切片文件 | 先确认传输完成，再允许发起打印 |
| JPEG，默认 6000 | 当前 A1 本地相机适配 | 不等于支持任意帧率或 RTSPS |
| 设备上报的 RTSPS URL | 当前 X2D 本地相机适配 | 使用实际上报地址，不拼接猜测路径 |

以上端口是当前 LayerHub 实现的默认值，不是永久稳定的厂商 API 保证。设备访问码、证书和身份校验应沿用受限配置；`get_access_code` 不是绕过认证的方法。

请求通常以 `print`、`system`、`camera`、`pushing`、`info`、`upgrade` 或 `xcam` 为外层对象，含 `command` 和字符串 `sequence_id`。参数类型保持原样，例如速度档位的 param 为字符串，轴方向为数值。不得把 UI 翻译应用到协议字段、槽位 ID 或枚举值。

相对移动采用单次、非保留发送，不因超时或断线自动重发。请求编号用于 LayerHub 去重，不能据此宣称固件保证“恰好执行一次”。

<a id="printing"></a>
## 4. 文件、选盘和打印控制

### 4.1 发起切片文件打印

`print/project_file` 是现有 LayerHub 的已切片 3MF 启动请求，必须配合已上传的设备内文件。官方 PrintJob 将参数交给网络代理；具体 JSON 归档以本项目 `internal/device/control.go` 的 `printPayload` 为依据，不能误标为公开源码中可直接找到的同名字面构造器。[官方 PrintJob][S-PrintJob]。

| 参数 | 类型 / 含义 |
| --- | --- |
| `param` | 选择的 `Metadata/plate_<盘号>.gcode`；盘号来自实际 3MF 内容，不强制视为连续编号 |
| `url` | 设备内文件 URL，现有项目使用 `ftp:///` 前缀；不是公网下载地址 |
| `subtask_name` | 本次任务标识，用于跟踪实际运行文件 |
| `bed_type` | 打印板类型，按切片和实物匹配 |
| `bed_levelling` | 是否请求打印前调平；字段拼写沿协议 |
| `flow_cali` | 是否请求相应流量校准；必须按机型能力处理 |
| `vibration_cali`、`layer_inspect` | 当前项目固定 false，不代表设备所有选项均已验收 |
| `timelapse` | 是否请求延时摄影，不等于开始实时视频传输 |
| `use_ams` | A1 裸机为 false；X2D 根据实际供料选择 |
| `ams_mapping` | 旧式映射数组；普通 AMS 路径通常由单元与槽位合成 |
| `ams_mapping2` | 新式每份耗材的 `ams_id` / `slot_id` 映射 |
| `project_id`、`profile_id`、`task_id`、`subtask_id` 等 | 当前代码的兼容占位字段；不是 LayerHub 数据库 ID 的直接映射 |

多盘 3MF 每次启动仍要明确选盘。LayerHub 将各盘登记为独立任务；队列顺序、人工取件与库存结算属于软件业务，不是打印机 MQTT 命令。不要因为文件有多盘，就推断设备可自动清板并连续开打。

`print/gcode_file` 的 `param` 是设备内 G-code 文件路径，官方也有将它用于旧式校准的分支。当前 Beta 上传仅接受已切片 3MF；存在此低层入口不代表产品允许上传任意原始 G-code。

### 4.2 运行控制

| 指令 | 主要参数 | 确认依据 / 项目边界 |
| --- | --- | --- |
| `print/pause` | `param` 为空字符串 | 新鲜任务状态进入暂停；不能只靠回执 |
| `print/resume` | 普通继续的 `param` 为空字符串 | 新鲜匹配任务回到运行；带告警信息的继续是另一分支 |
| `print/stop` | 普通停止的 `param` 为空字符串；部分官方分支带 `job_id` | 确认实际停止；不得记为成功完工或直接结算库存 |
| `print/print_speed` | `param` 为 `"1"`–`"4"` | 核对 `spd_lvl`；不是 mm/s，也不是任意百分比 |
| `print/skip_objects` | `obj_list` 对象编号数组 | 要有匹配切片对象标记和设备能力；不等于取消某盘或整个任务 |

速度枚举依次为静音、标准、运动、狂暴，现有 UI 显示 50%、100%、124%、166%。这只是档位表达，不保证每段运动或实际总时长按该比例变化。A1 / X2D 在本轮空闲测试中没有重新验证打印中的上述动作。[任务控制及速度][S-DeviceManager]。

<a id="settings"></a>
## 5. 温度、灯光、风扇和风道

### 5.1 温度与喷嘴

| 指令 / 路径 | 参数 | X2D | A1 |
| --- | --- | --- | --- |
| `print/set_nozzle_temp` | `extruder_index`、`target_temp` | 当前已适配 0 / 1 独立设置 | 不因源码有此入口就替换当前旧路径 |
| `print/gcode_line` 携带 `M104` | S 表示目标温度 | 双喷嘴必须明确对象，不能用它代替现有独立路径 | 当前喷嘴目标设置路径 |
| `print/set_bed_temp` | `temp` | 仅对应能力存在时使用 | 当前没有套用此新路径 |
| `print/gcode_line` 携带 `M140` | S 表示目标温度 | 无新式热床能力时的既有路径 | 当前热床设置路径 |
| `print/set_ctt` | `ctt_val` | 官方配置有腔温能力，项目未适配、未实测 | 不适用 |
| `print/select_extruder` | `extruder_index` | 候选；会切换工作对象，不是只切换显示页 | 不适用双喷嘴切换 |
| `print/refresh_nozzle` | 序列号等通用字段 | 刷新候选，不能推断它会完成实物更换 | 不自动作为 A1 热端更换功能开放 |

温度设为 0 表示目标加热关闭，不代表喷嘴 / 热床已经冷却至 0°C。确认目标值后，实际温度仍可能较高。双喷嘴反馈必须来自指定编号的新鲜字段，另一喷嘴更新不能确认本次命令。[温度与喷嘴控制][S-DeviceManager]、[喷嘴选择][S-DevCtrl]、[腔温][S-DevChamberCtrl]。

### 5.2 灯光

`system/ledctrl` 的主要字段为 `led_node`、`led_mode`、`led_on_time`、`led_off_time`、`loop_times`、`interval_time`。源码中有 `chamber_light` 与 `chamber_light2`，以及 on / off / flashing 等模式。不能因此为 A1 增加不存在的第二盏灯。

现有项目只把有对应证据的节点开放为开关。A1 的亮灭已由现场确认，但灯光遥测曾保持静态，因此界面使用“上次请求”，设备成功回执不表示有独立传感器确认了亮灭。闪烁及第二节点不在本轮实测范围。[灯光][S-DevLampCtrl]。

### 5.3 风扇与风道

| 路径 | 字段 / 编号 | 说明 |
| --- | --- | --- |
| 新式 `print/set_fan` | `fan_index`、`speed`（百分比） | 当前 X2D 路径；不能与旧式 0–255 值混用 |
| 旧式 `M106` | P 为风扇编号、S 为 0–255 | A1 当前仅模型风扇 P1；项目将百分比换算为 S |
| `print/set_airduct` | `modeId`、`submode` | 风道模式切换候选；会影响可手动调整的风扇，不是普通风速设置 |
| `print/close_air_filt` | 通用字段 | 关闭相关净化流程候选，不能替代所有风扇归零 |

X2D 当前适配的编号为 1（模型风扇）、2、3、10；本轮四路均验证 20%及关闭。现有 UI 对 2/3/10 的标签为辅助 1、排风、辅助 2；**官方 N6 配置中 2 / 10 的用途会随风道模式变化**，可能显示加热 / 过滤用途。后续应以实际模式和固件回报核对，不把静态标签当硬件永久定义。

只有设备实时 `modeCur` / `modeList` 允许的风扇才能手动调整。`airduct.parts[].id` 是位编码，不能把原始整个 id 直接当 `fan_index`。项目按相应位提取编号、范围与状态，并保留字段新鲜度。

已知问题：X2D 请求 25% 时实际回报 20%，原部署会进入待核对。本地修复将可选值限制为整十档，快捷值为关闭 / 20 / 50 / 80 / 100；不静默把 25 改成 20，也不放宽现代风扇成功容差。关闭要求新鲜回报精确为 0，非零请求不能由 0 确认。**这项修复已通过本地检查，尚未部署；除 20% / 关闭外，其余档位未逐项实测。**[风扇与风道][S-DevFan]、[X2D 配置][S-N6]。

<a id="motion"></a>
## 6. 归位、XYZ 与挤出

### 6.1 原生归位与点动

| 指令 | 参数 | 当前使用范围 |
| --- | --- | --- |
| `print/back_to_center` | 通用序列号 | X2D 已适配并实测；不能仅按名称假定所有机型三轴均完成回零 |
| `print/xyz_ctrl` | `axis` 为 X/Y/Z；`dir` 为 1 / -1；`mode` 为 0 / 1 | mode 0 / 1 对应官方小步 / 大步；LayerHub 目前仅开放小步，产品标注 1 mm |

官方 UI 提供 1 与 10 两档；10 mm 是大步选项，不是最大机械行程。当前 X2D 仅正向小步已现场逐轴确认，未用量具测定准确位移，未验收反向及 10 mm。

### 6.2 A1 旧式路径与方向

官方无对应原生能力时可经 `gcode_line` 发送归位和相对移动。归位分支存在空闲整机 `G28` 与打印中的 X 轴归位区别；**LayerHub 维护移动只允许空闲，不沿用打印中归位入口。**

旧式 XYZ 序列涉及 `M211` 限位上下文、`M1002` 参考模式上下文、`G91` 相对模式及 `G1` 运动，之后处理上下文恢复。这里记录组成，不提供可直接执行的拼接脚本。专有指令的细节不能直接套用通用 Marlin 语义，也不能以末尾强制 `G90` 代替恢复原状态。

| 官方按钮逻辑输入 | CoreXY 分支 | A1 的 i3 分支（源码推导，非本机实测） |
| --- | --- | --- |
| X 正 / 负输入 | 保留符号 | 保留符号 |
| Y 正 / 负输入 | 保留符号 | 反转符号；打印板的前后标签待现场核对 |
| Z 上按钮 | 官方输入为负 | 反转后为正轴值；对应 A1 横梁升降待实测 |
| Z 下按钮 | 官方输入为正 | 反转后为负轴值；不能照搬 X2D 热床标签 |

官方默认按钮速度参数中 X/Y 为 3000、Z 为 900；这不是推荐直接启用的项目验收值，也不是设备最大速度。协议参数、单位和物理效果应一起验证。不能在新式命令失败后自动改发旧式命令。[轴控制][S-DevAxisCtrl]、[按钮逻辑][S-StatusPanel]。

### 6.3 回零与结果确认

官方兼容逻辑把 `home_flag==0` 当可用回零状态；LayerHub 不采用该默认。未知、零值、过期、另一轴更新或成功回执，都不能直接证明三轴已回零。[官方回零兼容逻辑][S-DevAxisHeader]。

现有 X2D 归位观察最多约 80 秒，至少等待约一分钟，并核对发送后的新鲜三轴标记；这是本项目的保守确认策略，不是通用协议规定。点动返回接受后仍需现场确认已停稳、无异常。A1 若无法提供可靠回零证据，应先设计并验收明确的现场核对流程，不能降低现有锁定条件来开放。

### 6.4 少量挤出与完整装退料不同

旧式少量挤出使用相对 E 模式与 E 轴运动；官方新式入口为 `print/set_extrusion_length`，主要参数是 `extruder_index`、`length`。不能因为有原生 XYZ 能力就把 E 也塞进同一白名单。

官方界面检查实际温度至少 170°C 后才允许相应手动挤出，但它不是 PETG 等材质的推荐温度。后续还需实际材质、温度新鲜度、挤出模式副作用及现场观察。少量挤出 / 回抽不代表完成完整进料、切料或退料。两台本轮均未验证这类指令。[实际温度门槛与挤出入口][S-StatusPanel]、[新式挤出长度][S-DeviceManager]。

<a id="filament"></a>
## 7. 外置耗材与 AMS

### 7.1 装料、退料、恢复流程

| 指令 | 主要参数 | 适用边界 |
| --- | --- | --- |
| `print/ams_change_filament` | `ams_id`、`slot_id`、`target`、`curr_temp`、`tar_temp`，部分分支有 `extruder_id` | 同时存在外置料架与 AMS 分支；不是只有装 AMS 才可能使用 |
| `print/ams_control` | `param` 为 resume / reset / pause / done / abort 中的对应动作 | 控制供料流程，不等于打印任务的暂停 / 继续 / 停止 |
| `print/ams_reset` | 通用字段 | 重置相关供料状态候选；不能作为解除软件任务锁的捷径 |

旧式外置装料入口使用虚拟料架 254；退料目标为 255，新式结构还拆分料架、槽位及挤出机。普通 AMS 槽位在协议里通常从 0 编号，UI 第一格不应直接当协议 1。新式映射可能按高低位组合单元和槽位，不能只用 `单元×4+槽位` 处理所有新设备。

X2D 必须核对所选耗材、AMS 绑定和目标喷嘴；A1 裸机选外置路径，不能伪造第一组 AMS。执行完整流程可能需要加热、切料及用户插拔；没有流程结束和现场证据不能称完成。 2026-10-29 补充实测：X2D + AMS 2 Pro，主挤出机 1，PETG Basic 黄（槽 1）→蓝（槽 3）→白（槽 4）→退料已通过新鲜反馈和现场确认；仅使用四次 `ams_change_filament`，不代表 `ams_control done` 已验证。[装退料与流程参数][S-DeviceManager]。

### 7.2 耗材元信息、读取与设置

| 指令 | 参数 / 用途 | 备注 |
| --- | --- | --- |
| `print/ams_filament_setting` | `ams_id`、`slot_id`、`tray_id`、`tray_info_idx`、`setting_id`、`tray_color`、`tray_type`、喷嘴温度上下限；部分分支有 `cols` / `ctype` | 写入元信息候选，不会物理更换料卷 |
| `print/ams_get_rfid` | `ams_id`、`slot_id` | 只用于有对应读取硬件的单元；不能假设裸机 / AMS lite 支持 |
| `print/ams_user_setting` | `ams_id`、`startup_read_option`、`tray_read_option`、`calibrate_remain_flag` | 能力和单元类型须匹配 |
| `M620` 的源码分支 | C 校准、R 读取、P 选择等子参数 | 属于旧式候选，不作为自动失败回退路径 |

设备中的颜色通常是 RGBA 字符串，不应把显示用 RGB 截断规则当写入协议。设备料卷标签、料槽编号、打印文件配色索引和 LayerHub 料卷 ID 是不同编号空间。

设备估算余量百分比不等于已称量克数；裸机外置料架也不能凭元信息推断净剩余重量。遥测同步、预留、实际扣减和人工取件结算必须分开。

### 7.3 AMS 烘干

`print/ams_filament_drying` 的字段包括 `ams_id`、`mode`、`filament`、`temp`、`duration`、`humidity`、`rotate_tray`、`cooling_temp`、`close_power_conflict`。官方此入口按小时表达 duration，mode 应取与设备版本一致的枚举，不能凭常见数字猜测。停止沿同命令的 Off 分支，并清理对应控制参数。

`print/auto_stop_ams_dry` 是另一个自动停止相关入口，不能把它当所有设备通用的相同操作。AMS 原版、AMS 2 Pro、AMS HT、AMS lite 的能力不同：只有实物具备相应功能且设备允许时才适用。A1 裸机不适用，AMS lite 也不能因看到此命令就显示烘干按钮。本项目尚未开放或实测这些流程。[烘干][S-DevFilaSystemCtrl]。

### 7.4 X2D 喷嘴自动映射

`print/get_auto_nozzle_mapping` 涉及 `calibration`、`extrude_cali_manual_mode`、`filament_seq`、`ams_mapping` 以及后续每份耗材 / 喷嘴信息等复合结构。不能只提供一个“喷嘴编号”来替代完整准备信息。该能力属于后续候选，当前不声称完整适配；切片本身的喷嘴、直径、耗材和机型一致性仍须检查。[自动映射][S-DevMappingNozzle]。

<a id="calibration"></a>
## 8. 校准与打印选项

### 8.1 整机校准与材料校准

`print/calibration` 的 option 是位掩码。固定源码的字段含义如下，**仅表示参数定义，不表示两款机器都支持所有位**。

| 位 | 源码选项 | 本项目处理边界 |
| --- | --- | --- |
| 0 | `xcam_cali` | 有相机不等于支持此校准；不按品牌统一开启 |
| 1 | `bed_leveling` | 按设备调平能力与实物状态验收 |
| 2 | `vibration` | 与普通 XYZ 点动不同的长动作，单独验收 |
| 3 | `motor_noise` | A1 配置标记支持；X2D 此固定配置标记 false |
| 4 | `nozzle_cali` | X2D 配置有喷嘴偏移校准；A1 不照搬双喷嘴项目 |
| 5 | `bed_cali` | X2D 配置有高温热床校准；不能直接套用到 A1 |
| 6 | `clumppos_cali` | 能力候选，不能因 option 有这一位而启用 |

材料校准入口还包括 `extrusion_cali`、`extrusion_cali_set`、`extrusion_cali_get`、`extrusion_cali_del`、`extrusion_cali_sel`、`extrusion_cali_get_result`、`flowrate_cali`、`flowrate_get_result`。涉及耗材、喷嘴直径 / 类型、校准编号和分支嵌套数据，不归并成一个统一 JSON 模板。

“流量校准”“自动流量校准”和某条旧式挤出校准入口不是同义能力：A1 官方配置中自动流量校准标记 false，而 X2D 的旧式 `support_extrusion_cali` 标记 false。必须按所用分支检查，不能只看名称里有 calibration 就判断可用。以上本轮都未实测。[校准构造][S-DeviceManager]、[X2D 配置][S-N6]、[A1 配置][S-N2S]。

### 8.2 检测与其他打印选项

| 指令 | 参数 / 子选项 | 备注 |
| --- | --- | --- |
| `print/print_option` | `auto_recovery`、`auto_switch_filament`、`air_print_detect` | 恢复、耗材备份及检测开关按独立能力处理 |
| 同上 | `sound_enable`、`filament_tangle_detect` | A1 / X2D 的提示音和检测能力不同 |
| 同上 | `nozzle_blob_detect`、`nozzle_blob_detect_v2` | v2 的 mode 不是布尔值；按对应枚举处理 |
| 同上 | `air_purification` | 净化方式，不适用于没有腔体 / 相关硬件的 A1 |
| `xcam/xcam_control_set` | `module_name`、`control`；旧分支有 `enable`、`print_halt`，可带灵敏度 | 模块名按型号和固件能力核实，不直接复制其他机型列表 |
| `print/set_against_continued_heating_mode` | `enable` | 持续加热防护相关候选，不能当普通温度设置 |
| `system/set_door_stat` | `config` 0 / 1 / 2：关闭 / 警告 / 暂停策略 | 候选；不适用于没有门的 A1 |
| `system/print_cache_set` | `config` 布尔值 | 远程打印文件保存策略候选 |

这些选项均不属于本轮低影响控制实测。某些选项会影响打印中的处理策略，不能作为无副作用“读取”发送。[打印选项][S-DevPrintOptions]、[门与文件缓存][S-DeviceManager]。

<a id="camera"></a>
## 9. 相机、录像和照片

| MQTT 指令 | 参数要点 | 不能混淆的含义 |
| --- | --- | --- |
| `camera/ipcam_record_set` | `control` 为 enable / disable | 设备录像，不是开启网页视频通道 |
| `camera/ipcam_timelapse` | `control` 为 enable / disable | 延时摄影选项，不是实时帧率设置 |
| `camera/ipcam_resolution_set` | `resolution` | 必须采用本机报告支持的枚举；不能强制 A1 提升源分辨率 |
| `camera/ipcam_get_media_info` | `sub_command`、`storage`、`total_layer` | 固定源码示例查询延时摄影空间 |
| `camera/ipcam_delete_oldest_timelapse` | `storage`、`total_layer` | 会删除内容，不能作为普通空间查询替代 |
| `camera/ipcam_cap_pic_set` | `control` 为 enable / disable | 固定源码中的拍照选项，不应冒充“一次即时拍照”指令 |

当前 LayerHub 相机画面和照片由独立视频 / JPEG 通道读取并保存；后端截图功能不等于向打印机发送上述拍照选项。X2D / A1 已有接入记录，但相机这组 MQTT 设置没有全部实测。

A1 官方配置列出 720p，相机源的低帧率不是把界面写成更高帧率就能改善。X2D 配置与通道不同，不能照搬 RTSPS 到 A1。录像设置能力、存储空间和视频传输协议要分别判断。[录像及媒体][S-DeviceManager]、[拍照选项][S-DevPrintOptions]、[机型配置][S-N2S]。

<a id="maintenance"></a>
## 10. 告警、诊断和升级

### 10.1 查询与状态推送

| 指令 | 参数 | 说明 |
| --- | --- | --- |
| `info/get_version` | 通用序列号 | 固件 / 模块版本查询候选 |
| `pushing/pushall` | `version`、`push_target`；当前代码常用 1 / 1 | 请求完整状态，不是执行打印 |
| `pushing/start`、`pushing/stop` | 通用序列号 | 官方状态推送入口；与打印 start / stop 分开 |
| `system/get_access_code` | 通用序列号 | 敏感返回内容须受限处理；不是无需认证获取凭据 |

查询也需要节流。`pushall` 返回的匹配字段可以用来补充状态，但不能把命令回显当成物理动作完成。当前 LayerHub 已使用状态推送 / 完整状态读取，并不会因资料归档而新发上述命令。[查询与推送][S-DeviceManager]。

### 10.2 告警处理入口

| 指令 / 分支 | 参数要点 | 边界 |
| --- | --- | --- |
| `print/clean_print_error` | `subtask_id`、`print_error` | 清除错误显示 / 记录不等于故障已经排除 |
| `system/uiop` | 固定分支有 `name`、`action`、`source`、`type`、`err` | 处理相应错误对话框，不代表维修完成 |
| `print/idle_ignore` | `err`、`type` | 特定空闲错误处理候选，不自动忽略 |
| `print/ignore` | `err`、`param`、`job_id` | 与当前告警、任务匹配后才可讨论适配 |
| `print/resume` / `print/stop` 的 HMS 分支 | 可带 `err`、`job_id`、特定 `param` | 不是普通任务控制请求的通用替代模板 |
| `print/buzzer_ctrl` | `mode`（固定源码停止蜂鸣分支为 0） | A1 有提示音能力配置；X2D 固定配置不支持，均未在本轮测试 |

授权校验告警不得通过清错或忽略来绕过。打印机停止、成功完工、人工取件和耗材结算是不同状态；清告警不能直接把业务任务改为完成。[告警构造][S-DeviceManager]。

### 10.3 固件与附件升级（仅归档）

`upgrade/upgrade_confirm`、`upgrade/consistency_confirm` 包含 `src_id`；`upgrade/start` 包含 `url`、`module`、`version`、`src_id`。`upgrade/mc_for_ams_firmware_upgrade` 还有附件固件选择参数。它们不是 LayerHub 软件升级接口，当前不开放，也未实测。[设备升级][S-DevUpgradeCtrl]、[AMS 固件入口][S-DevFilaAmsSettingCtrl]。

源码还有喷嘴架专用的 `nozzle_holder_ctrl`、`nozzle_info_confirm`、`holder_nozzle_refresh`、`upgrade/wtm_upgrade`。它们出自独立喷嘴架流程，**不能因 X2D 有两个喷嘴就当作该机已支持**，也不用于 A1。附录保留其排除说明。恢复出厂、任意重启或网络重配并未在本次范围获得足够的适用证据，不编造命令或参数。

<a id="layerhub"></a>
## 11. LayerHub 接口与结果确认

以下是软件业务 API，不是直接发给打印机的 MQTT 命令。使用时仍受登录、权限、同源和 CSRF 等约束；文档没有包含登录凭据。

| 接口 | 请求要点 | 说明 |
| --- | --- | --- |
| `POST /api/printers/settings` | `printerId`、`key`、`value` | 设置温度 / 灯光 / 风扇等，由服务选择实际协议 |
| 同上，`key=resolve_settings` | 核对动作 | 解除对应未确认设置记录；不重发设备设置 |
| `POST /api/printers/motion` | `printerId`、`action`、`requestId`、`clear`；点动有 `axis`、`direction`、`step` | 当前只开放 X2D 1 mm 点动和归位 |
| 同上，`action=review` | `commandId` 及核对上下文 | 现场确认后的结果核对，不产生新的物理移动 |
| `POST /api/printers/control` | 按当前 `ControlInput` 传参 | 暂停 / 继续 / 停止，校验当前任务归属与状态 |
| `GET /api/printers/commands` | `printerId` | 审计与待确认记录 |
| `GET /api/printers/camera` | 按当前相机接口参数 | 相机内容；不自动等同相机 MQTT 设置 |
| `POST /api/jobs/send`、`/api/jobs/start`、`/api/jobs/dispatch` | `jobId` 等当前业务参数 | 传输、启动、组合调度；必须保证不重复启动 |
| `POST /api/jobs/complete` | 当前业务结算参数 | 人工登记，不是打印机的停止或成功完成指令 |

设置 key：X2D 为 `nozzle:0`、`nozzle:1`、`bed`、`speed`、实际可用灯光节点以及 `fan:1/2/3/10`；A1 为 `nozzle`、`bed`、`speed`、`chamber_light`、`part_fan`。未提供有效设置读数的项目不可仅靠拼接 key 发出。

| 软件结果 | 应怎样解释 |
| --- | --- |
| `pending` | 尚在等待，不重复提交 |
| `succeeded` | 达到对应代码的确认条件；不是所有物理质量指标都通过 |
| `accepted` | 设备已接受；例如 A1 灯光仍需现场 / 相机确认实际亮灭 |
| `uncertain` | 结果不明，保留核对锁，禁止自动重试 |
| `rejected` | 设备明确拒绝，处理原因，不自动换协议绕过 |
| `reviewed` | 人工核对已记录；不是重新执行或伪造设备成功回执 |

运行中的任务、传输、待取件、待结算和未确认动作会影响可操作性。不能仅因为遥测是 IDLE / FINISH 就直接解除业务锁。移动需管理员、状态新鲜、路径清空、无告警及同机互斥；A1 尚未因本次研究获得移动授权或能力开放。

<a id="validation"></a>
## 12. 实机验证记录与后续验收

下表是 2026-10-29 已保存的有限范围证据，不等于两款机型全部指令已经验收。只记录结果，不包含私有连接资料。

| 项目 | X2D | A1 |
| --- | --- | --- |
| 灯光 | 亮灭与反馈检查，恢复原状态 | 现场分别确认亮 / 灭；使用接受回执表达 |
| 喷嘴目标 | 编号 0 / 1 各 50°C 及归零 | 50°C 及归零 |
| 热床目标 | 40°C 及归零 | 40°C 及归零 |
| 风扇 | 四路分别 20%及归零；模型风扇有现场转动确认 | 模型风扇 20%及归零反馈 |
| 归位 | 原生回执及新鲜三轴标记；现场确认停稳无异常 | 旧式 G28 + M400；新鲜低三位 0→7，现场确认停稳无异常 |
| X/Y/Z 小步 | 每轴一次正向小步，用户逐次确认看到移动、停稳无异常 | X 正向小步现场看到向右，停稳无异常；Y 首次及追加慢速三段未看清，暂停待核对；Z 未发送 |
| AMS 完整装换退料 | AMS 2 Pro、主挤出机 1、PETG Basic：黄 1 → 蓝 3 → 白 4 → 退料，现场逐色确认通过 | 未验证 |
| 25% 风扇 | 回报 20%，触发原版待核对；人工确认后关闭，未重复发送 | 未据此推断同样行为 |

本轮结束时两台均在线空闲、无告警或 pending/uncertain；加热目标与测试风扇归零，灯光恢复原设置。没有新启动打印、取件结算或库存扣减。温度测试验证目标设置与反馈，不证明稳态温度精度。

尚未验证：本轮打印中的暂停 / 继续 / 停止 / 速度切换，反向及 10 mm 移动、A1 的 Y 结果确认及 Z 轴、少量挤出、A1 完整装退料及其他外置/供料器组合、喷嘴切换、烘干、校准、相机设置及升级。项目历史上有真实打印记录，但不能据此把上面所有分支标成已测。

建议后续顺序：A1 独立适配的离线检查 → 有人在场的归位 → 每轴正反小步 → 大步 → 单独进退料；打印过程控制放到下一次授权的正常 / 可中断测试件中。每个结果记录机型、固件、附件、原始参数、命令 ID、独立新鲜反馈和现场确认。未收到新授权时不因本手册自动执行这些步骤。

<a id="index"></a>
## 13. 完整命令索引

本节由固定源码中的命令构造位置逐项核对：64 个固定分区 / 命令项，另补 `pushing/start`、`pushing/stop` 两个动态白名单项和 LayerHub 的 `print/project_file`，共 67 项。包含候选和排除项，**不是“已支持 67 项”**。动态告警回覆仍须按实际消息和固件处理，未公开固件接口不在此完整性声明内。

| # | 分区 / 命令 | 参数摘要（分支分别核对） | 机型 / 实现与验证边界 | 出处 |
| --- | --- | --- | --- | --- |
| 1 | `camera/ipcam_cap_pic_set` | control | 两机能力候选；拍照选项，非即时截图 | [DevPrintOptions.cpp:543](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevPrintOptions.cpp#L543) |
| 2 | `camera/ipcam_delete_oldest_timelapse` | storage, total_layer | 条件适用；删除内容，未适配 | [DeviceManager.cpp:2186](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L2186) |
| 3 | `camera/ipcam_get_media_info` | sub_command, storage, total_layer | 条件适用；媒体空间查询候选 | [DeviceManager.cpp:2175](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L2175) |
| 4 | `camera/ipcam_record_set` | control | 录像选项候选；不是实时视频连接 | [DeviceManager.cpp:2134](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L2134) |
| 5 | `camera/ipcam_resolution_set` | resolution | 按本机分辨率枚举；未适配 | [DeviceManager.cpp:2156](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L2156) |
| 6 | `camera/ipcam_timelapse` | control | 两机能力候选；未单独实测 | [DeviceManager.cpp:2145](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L2145) |
| 7 | `info/get_version` | sequence_id | 两机版本查询候选 | [DeviceManager.cpp:1261](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L1261) |
| 8 | `print/ams_change_filament` | ams_id, slot_id, target, curr_temp, tar_temp, extruder_id（分支） | 两机供料候选；裸机走外置分支 | [DeviceManager.cpp:1635](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L1635) |
| 9 | `print/ams_control` | param | 供料流程动作候选，非打印任务控制 | [DeviceManager.cpp:1757](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L1757) |
| 10 | `print/ams_filament_drying` | ams_id, mode, filament, temp, duration 等 | X2D 配烘干型 AMS 才可讨论；A1 裸机不适用 | [DevFilaSystemCtrl.cpp:28](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevFilaSystemCtrl.cpp#L28) |
| 11 | `print/ams_filament_setting` | ams_id, slot_id, tray_id, tray_type, tray_color 等 | 料槽/外置元信息候选；不等于库存克数 | [DeviceManager.cpp:1705](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L1705) |
| 12 | `print/ams_get_rfid` | ams_id, slot_id | 相应 AMS 硬件条件；A1 裸机不适用 | [DeviceManager.cpp:1735](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L1735) |
| 13 | `print/ams_reset` | sequence_id | 供料重置候选；不用于解除业务锁 | [DevFilaSystemCtrl.cpp:14](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevFilaSystemCtrl.cpp#L14) |
| 14 | `print/ams_user_setting` | ams_id, startup_read_option, tray_read_option, calibrate_remain_flag | AMS 硬件条件；A1 裸机不适用 | [DeviceManager.cpp:1667](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L1667) |
| 15 | `print/auto_stop_ams_dry` | sequence_id | 按烘干单元和固件核对；未适配 | [DeviceManager.cpp:1768](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L1768) |
| 16 | `print/back_to_center` | sequence_id | X2D 归位已实测；A1 原生能力未确认 | [DevAxisCtrl.cpp:13](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevAxisCtrl.cpp#L13) |
| 17 | `print/buzzer_ctrl` | mode | A1 能力候选；X2D 配置不支持提示音 | [DeviceManager.cpp:1551](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L1551) |
| 18 | `print/calibration` | option | 两机按独立能力位筛选，未实测 | [DeviceManager.cpp:1904](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L1904) |
| 19 | `print/clean_print_error` | subtask_id, print_error | 两机错误处理候选；不等于故障排除 | [DeviceManager.cpp:1369](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L1369) |
| 20 | `print/close_air_filt` | sequence_id | X2D 净化候选；A1 不适用 | [DeviceManager.cpp:1502](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L1502) |
| 21 | `print/extrusion_cali` | tray_id / filaments / nozzle_diameter / mode 等分支 | 材料校准候选；不能按名称统一开放 | [DeviceManager.cpp:1779](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L1779) |
| 22 | `print/extrusion_cali_del` | filament_id, cali_idx, nozzle_diameter 等 | 删除校准数据候选，未适配 | [DeviceManager.cpp:2002](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L2002) |
| 23 | `print/extrusion_cali_get` | filament_id, extruder_id / nozzle_id 等条件字段 | 校准表查询候选，未适配 | [DeviceManager.cpp:2030](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L2030) |
| 24 | `print/extrusion_cali_get_result` | nozzle_diameter | 校准结果查询候选，未适配 | [DeviceManager.cpp:2051](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L2051) |
| 25 | `print/extrusion_cali_sel` | tray_id, ams_id, slot_id, cali_idx 等 | 选用校准数据候选，未适配 | [DeviceManager.cpp:2061](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L2061) |
| 26 | `print/extrusion_cali_set` | tray_id / filaments / k_value / n_coef 等分支 | 写入校准值候选，未适配 | [DeviceManager.cpp:1805](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L1805) |
| 27 | `print/flowrate_cali` | tray_id, nozzle_diameter, filaments | 流量校准候选；按机型和分支核对 | [DeviceManager.cpp:2083](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L2083) |
| 28 | `print/flowrate_get_result` | nozzle_diameter | 流量结果查询候选，未适配 | [DeviceManager.cpp:2124](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L2124) |
| 29 | `print/gcode_file` | param | 设备内文件入口；Beta 不开放原始 G-code 上传 | [DeviceManager.cpp:1898](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L1898) |
| 30 | `print/gcode_line` | param | 两机经白名单设置使用；不开放任意输入 | [DeviceManager.cpp:3784](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L3784) |
| 31 | `print/get_auto_nozzle_mapping` | calibration, extrude_cali_manual_mode, filament_seq, ams_mapping 等 | X2D 自动喷嘴映射候选；A1 单喷嘴不直接套用 | [DevMappingNozzle.cpp:69](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevMappingNozzle.cpp#L69) |
| 32 | `print/holder_nozzle_refresh` | 喷嘴架相关分支参数 | 当前两机排除：独立喷嘴架专用 | [DevNozzleRackCtrl.cpp:101](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevNozzleRackCtrl.cpp#L101) |
| 33 | `print/idle_ignore` | err, type | 空闲告警处理候选；不自动忽略 | [DeviceManager.cpp:1453](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L1453) |
| 34 | `print/ignore` | err, param, job_id | 告警处理候选；不自动忽略 | [DeviceManager.cpp:1479](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L1479) |
| 35 | `print/nozzle_holder_ctrl` | 喷嘴架相关分支参数 | 当前两机排除：独立喷嘴架专用 | [DevNozzleRackCtrl.cpp:35](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevNozzleRackCtrl.cpp#L35) |
| 36 | `print/nozzle_info_confirm` | 喷嘴架相关分支参数 | 当前两机排除：喷嘴架确认流程 | [DevNozzleRackCtrl.cpp:83](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevNozzleRackCtrl.cpp#L83) |
| 37 | `print/pause` | param | 两机通用实现；本轮未做运行态复验 | [DeviceManager.cpp:1429](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L1429) |
| 38 | `print/print_option` | option / auto_recovery / sound_enable 等独立分支 | 按型号检测选项；未完整适配 | [DeviceManager.cpp:1836](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L1836) |
| 39 | `print/print_speed` | param（字符串 1–4） | 两机已实现；本轮未做运行态实测 | [DeviceManager.cpp:1825](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L1825) |
| 40 | `print/project_file` | param, url, subtask_name, use_ams, ams_mapping 等 | LayerHub 已实现；须文件、盘号和任务匹配 | [LayerHub printPayload](https://github.com/Tuang-Gaashuan/LayerHub/blob/b4d63353d69e2d6dc0a29e92f63df27b9ef259b9/source/internal/device/control.go#L37) |
| 41 | `print/refresh_nozzle` | sequence_id | 喷嘴刷新候选；不等于实物更换完成 | [DeviceManager.cpp:1612](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L1612) |
| 42 | `print/resume` | param；HMS 分支还有 err, job_id | 两机通用实现；告警分支分别核对 | [DeviceManager.cpp:1441](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L1441) |
| 43 | `print/select_extruder` | extruder_index | X2D 候选；A1 不适用双喷嘴切换 | [DevCtrl.cpp:63](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevCtrl.cpp#L63) |
| 44 | `print/set_against_continued_heating_mode` | enable | 防护选项候选；按能力核对 | [DevPrintOptions.cpp:474](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevPrintOptions.cpp#L474) |
| 45 | `print/set_airduct` | modeId, submode | X2D 候选；A1 不适用此新式风道 | [DevFan.cpp:112](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevFan.cpp#L112) |
| 46 | `print/set_bed_temp` | temp | X2D 有能力时使用；A1 当前用旧路径 | [DeviceManager.cpp:1563](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L1563) |
| 47 | `print/set_ctt` | ctt_val | X2D 腔温候选；A1 不适用 | [DevChamberCtrl.cpp:10](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevChamberCtrl.cpp#L10) |
| 48 | `print/set_extrusion_length` | extruder_index, length | 新式挤出候选，未开放；不替代整套装料 | [DeviceManager.cpp:1874](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L1874) |
| 49 | `print/set_fan` | fan_index, speed | X2D 四路 20%/关闭已测；A1 当前用旧路径 | [DevFan.cpp:80](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevFan.cpp#L80) |
| 50 | `print/set_nozzle_temp` | extruder_index, target_temp | X2D 双喷嘴目标已测；A1 当前用旧路径 | [DeviceManager.cpp:1602](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L1602) |
| 51 | `print/skip_objects` | obj_list | 对象标记和设备能力条件，未适配 | [DeviceManager.cpp:1398](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L1398) |
| 52 | `print/stop` | param；取消/告警分支有 job_id, err | 两机通用实现；停止不等于成功完工 | [DeviceManager.cpp:1408](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L1408) |
| 53 | `print/xyz_ctrl` | axis, dir, mode | X2D 正向小步已测；A1 未开放 | [DevAxisCtrl.cpp:32](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevAxisCtrl.cpp#L32) |
| 54 | `pushing/pushall` | version, push_target | 状态请求已有使用；需节流 | [DeviceManager.cpp:1338](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L1338) |
| 55 | `pushing/start` | sequence_id | 动态白名单：开始状态推送，非开始打印 | [DeviceManager.cpp:1356](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L1356) |
| 56 | `pushing/stop` | sequence_id | 动态白名单：停止推送，非停止打印 | [DeviceManager.cpp:1356](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L1356) |
| 57 | `system/get_access_code` | sequence_id | 敏感查询候选；不用于绕过认证 | [DeviceManager.cpp:1270](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L1270) |
| 58 | `system/ledctrl` | led_node, led_mode, led_on_time, led_off_time, loop_times, interval_time | 两机实际节点已实测；A1 使用接受回执 | [DevLampCtrl.cpp:39](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevLampCtrl.cpp#L39) |
| 59 | `system/print_cache_set` | config（布尔） | 保存打印文件策略候选，未适配 | [DeviceManager.cpp:4939](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L4939) |
| 60 | `system/set_door_stat` | config（0/1/2） | X2D 候选；A1 无门，不适用 | [DeviceManager.cpp:4917](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L4917) |
| 61 | `system/uiop` | name, action, source, type, err | 告警对话框处理候选，不等于维修完成 | [DeviceManager.cpp:1380](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp#L1380) |
| 62 | `upgrade/consistency_confirm` | src_id | 设备升级候选，未适配 | [DevUpgradeCtrl.cpp:20](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevUpgradeCtrl.cpp#L20) |
| 63 | `upgrade/mc_for_ams_firmware_upgrade` | src_id, id | AMS 附件固件入口候选；裸机不适用 | [DevFilaAmsSettingCtrl.cpp:16](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevFilaAmsSettingCtrl.cpp#L16) |
| 64 | `upgrade/start` | url, module, version, src_id | 设备/模块升级候选，未适配 | [DevUpgradeCtrl.cpp:33](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevUpgradeCtrl.cpp#L33) |
| 65 | `upgrade/upgrade_confirm` | src_id | 设备升级确认候选，未适配 | [DevUpgradeCtrl.cpp:11](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevUpgradeCtrl.cpp#L11) |
| 66 | `upgrade/wtm_upgrade` | 喷嘴架升级相关分支参数 | 当前两机排除：喷嘴架固件入口 | [DevNozzleRackCtrl.cpp:276](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevNozzleRackCtrl.cpp#L276) |
| 67 | `xcam/xcam_control_set` | module_name, control / enable / print_halt, halt_print_sensitivity | 检测模块能力条件；A1 不套用 X2D AI 模块 | [DevPrintOptions.cpp:459](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevPrintOptions.cpp#L459) |

<a id="sources"></a>
## 14. 来源与维护方法

官方资料固定到 Bambu Studio 提交 `79df7ae309be398ed30c9f397c13c9911c2edc57`。本次集中收集并校验 60 份公开源码 / 机型配置，结合此前头文件研究、LayerHub 源码和已保存实测记录。原始公开文件的 Git blob SHA1 / SHA256 索引保存在本地 QA；本手册不依赖那些目录才能阅读。

后续更新此文件时：保留日期与源码提交；修改参数前核对构造函数及调用条件；把“新增实现”和“新增实测证据”分别登记；设备固件或附件变化后重新确认适用性。只追加文件不应触发控制命令、升级、任务结算或库存变更。

本项目核对位置：`source/internal/device/control.go`、`equipment.go`、`x2d_settings.go`、`motion.go`、`camera.go`、`manager.go`，以及 `source/internal/app/equipment.go`、`motion.go`、`http.go`。这些项目路径用于维护定位；核心内容已在正文保留。

English reading note: This is a source-pinned command reference for X2D and bare A1, including protocol fields, capability differences, implementation status and bounded hardware evidence. The 67-entry index includes unsupported candidates and explicit exclusions; it does not claim 67 tested controls or cover unpublished firmware interfaces. No NAS/printer connection, physical command, deployment or inventory mutation was performed to produce this document.

[S-N6]: https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/resources/printers/N6.json
[S-N2S]: https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/resources/printers/N2S.json
[S-DeviceManager]: https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceManager.cpp
[S-PrintJob]: https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/Jobs/PrintJob.cpp
[S-StatusPanel]: https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/StatusPanel.cpp
[S-DevAxisHeader]: https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevAxis.h
[S-DevAxis]: https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevAxis.cpp
[S-DevAxisCtrl]: https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevAxisCtrl.cpp
[S-DevCtrl]: https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevCtrl.cpp
[S-DevChamberCtrl]: https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevChamberCtrl.cpp
[S-DevLampCtrl]: https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevLampCtrl.cpp
[S-DevFan]: https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevFan.cpp
[S-DevFilaSystemCtrl]: https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevFilaSystemCtrl.cpp
[S-DevMappingNozzle]: https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevMappingNozzle.cpp
[S-DevPrintOptions]: https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevPrintOptions.cpp
[S-DevUpgradeCtrl]: https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevUpgradeCtrl.cpp
[S-DevFilaAmsSettingCtrl]: https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevFilaAmsSettingCtrl.cpp
