# 手动移动 / Manual movement

Beta 维护控制的首版范围：**X2D、原生归位、XYZ 单次 1 mm 点动**。推荐使用 Bambu Studio 切片，LayerHub 管理团队、文件、排队及设备维护。

打开设备控制台的「手动移动」页。仅管理员可操作；先完成本机任务的取件和用量登记，在现场确认打印板和运动路径清空，再勾选确认。没有可靠坐标时显示未提供，不把坐标写成 0。

- 三轴回零和能力字段必须新鲜；没有回零证据时禁用点动。负数的 `home_flag` 按固件的 32 位标记解析，0 不推断为全部回零。
- 归位单独确认，观察窗口为最多 80 秒，至少等约一分钟并核对三轴标记。较晚到达的 X 轴标记不会被 25 秒超时过早判断。
- 每次单击只发一次原生指令。没有长按连动、全局方向键、自动重新归位或任意 G-code 输入。
- 点动回执表示接收，不能用它声称准确位移或物理动作完成；每次点动后要现场确认已停稳、无异常，再解除待核对锁。未收到确认时不自动重发。
- 移动与打印启动、传输、设置共用设备指令互斥。请求编号在现有审计中持久保存，重放同一编号不会重复发送动作。浏览器断线后的核对锁也会保留。
- 10 mm、挤出及其他型号暂未开放。打印、暂停、待取件、告警、离线、状态过期或结果未核对时禁用。

2026-10-28 实机记录：X2D 固件 01.02.00.00，原生归位回执成功，三轴回零标记最终均到位。X/Y/Z 正方向各一次 mode=0 小步指令返回成功，现场用户逐次确认看到了小幅移动、已停稳且无异常。未用量具测定准确 1 mm，未做 10 mm、极限行程或其他型号的实机验收；反方向参数有离线协议检查，未独立声称全部机械工况通过。照片和私人设备证据保存在本地 QA，不随源码公开。

## English

The initial Beta maintenance controls support **X2D native homing and individual 1 mm XYZ jog requests**. Open the printer console's Manual movement tab. Administrators must complete pickup/usage registration and confirm onsite clearance before use.

Motion requires fresh capabilities and confirmed homing. Missing coordinates remain unavailable. Signed 32-bit home flags are decoded without interpreting zero as fully homed. Homing has an 80-second observation timeout and waits at least about a minute before confirming all three flags, allowing delayed X-axis observations.

One click sends one non-retained QoS 0 native request. Request IDs persist in the existing audit; repeating the same ID does not resend movement. No hold-to-repeat, arbitrary G-code, automatic homing fallback or uncertain-command retry is implemented. Motion shares the command lock with dispatch and settings. An accepted jog still needs explicit onsite confirmation that motion has stopped without issues. Network uncertainty remains locked for review.

Hardware evidence on October 28, 2026: X2D firmware 01.02.00.00 accepted native homing; all three homing flags eventually arrived. One positive-direction small-step request on each axis was accepted and individually confirmed onsite as visible motion, stopped, without issues. Exact millimetres were not measured. 10 mm, extrusion, travel limits and other models have not been validated. Negative-direction encoding is checked offline without claiming every physical direction or condition has been tested.

Protocol references: [official axis controls](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevAxisCtrl.cpp), [official axis status](https://github.com/bambulab/BambuStudio/blob/79df7ae309be398ed30c9f397c13c9911c2edc57/src/slic3r/GUI/DeviceCore/DevAxis.h).
