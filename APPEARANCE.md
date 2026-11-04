# 个性化显示 / Appearance

2026-10-29 已部署到现有云端与 NAS；尚未上传 GitHub 或打包新版本。

头像 / 账户设置 → 个性化显示。支持蓝、绿、青、紫、橙、玫红六种主题色，或完整的 `#RRGGBB` 自定义颜色；过浅颜色会加深按钮色，以保持白色文字清晰。设备状态及实际耗材配色不跟随主题色。

界面模式：浅色、深色、跟随系统。页面密度：宽松、舒适、紧凑，与字号独立。现有二十种字体选项，字号、字重、文字对比度继续保留。默认仍为浅色蓝色、舒适密度。

选择时工作区实时预览，设备数据刷新不会冲掉草稿。保存后才写入当前浏览器、当前账户的偏好；取消、关闭和 Escape 恢复已保存设置。恢复默认也是草稿，保存才生效。切换账户丢弃上一账户草稿。浏览器禁止存储时显示错误，不伪称已保存。

当前不提供云端跨设备同步。手机、电脑或其他浏览器分别记忆。修改外观不会请求设备控制、任务操作或库存写入接口。操作系统要求减少动画时，尊重该偏好。

开发维护：修改界面 CSS 后运行 `node source/qa/build-themes.cjs`；更新文案后运行 `node source/qa/build-locales.cjs`。CI 检查两份生成结果。深色与主题覆盖通过 CSS 语法树生成，保留媒体条件，排除实际耗材色与模型插画；不得用整页滤镜改变相机或料卷颜色。

## English

Deployed to the existing cloud and NAS installation on 2026-10-29; not yet uploaded to GitHub or packaged as a new release. Open Account → Appearance. Choose six presets or a validated hex colour, light/dark/system mode, and density independently of text size. All ten existing fonts remain available, with ten additional local font choices (20 total). Pale custom accents are darkened for readable white button text. Status and filament colours remain independent.

The workspace previews changes immediately, including while telemetry refreshes. Save persists preferences per account in this browser. Cancel/close/Escape restore saved preferences; reset is a reversible draft. Account changes discard the previous draft. Storage errors do not claim success. Preferences do not sync across devices, and appearance changes send no device/business writes.

Regenerate themes after CSS edits and locales after text edits using the commands above. CI validates generated files. No whole-page colour filter is used; respect OS reduced-motion preferences.

## 新增本机字体 / Additional local fonts

华文中宋（推荐）、华文宋体、华文细黑、华文楷体、幼圆、隶书、华文琥珀、Bahnschrift、主题字体（Bahnschrift + 微软雅黑）和等宽（数据感）。已有宋体、黑体等选项不重复添加，不修改默认字体或既有偏好。

These ten additional choices use locally installed fonts. The themed combination uses Bahnschrift for Latin text and Microsoft YaHei for Chinese. Monospace uses Cascadia Code or Consolas for Latin text. Font files are not copied or bundled. Devices without these families use the fallback stack described in settings; appearance can differ across Windows, macOS and mobile devices.

这些选项依赖当前设备已安装的字体，未安装时使用设置说明中的后备字体；不同电脑和手机可能呈现不同效果。不会上传、复制或打包本机字体文件。
