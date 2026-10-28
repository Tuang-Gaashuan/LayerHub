# Local Bambu Studio shortcut / 本机 Bambu Studio 快捷入口

## English

The first sidebar item requests local Bambu Studio, independently of NAS/printer reachability. Use **Personal account → Local Bambu Studio** to edit its Windows path.

1. Find `bambu-studio.exe` (or `BambuStudio.exe`) in File Explorer and copy its full local path, including the drive letter. Paths with spaces, Chinese characters and surrounding copy-path quotes are accepted.
2. Choose **Save and download setup**. Double-click `LayerHub-Bambu-Studio.reg` and explicitly confirm importing it for your current Windows user.
3. Click **Bambu Studio** in the sidebar and confirm the browser's external-app prompt. If an embedded browser blocks opening it, visit your LayerHub site in Edge or Chrome.

The page cannot run an arbitrary EXE path, inspect your filesystem, detect the registry association, or confirm a successful desktop launch. It reports a launch request. Repeat the import after changing the executable path. This workflow targets Windows; it is not a macOS/Linux installer or a desktop client.

The path stays in this browser's local storage; it is never sent to the NAS or cloud. The generated registration affects only `HKEY_CURRENT_USER\Software\Classes\layerhub-studio` and quotes one fixed executable path. The handler does not use a command shell or forward URI arguments/files. No resident agent, local HTTP server or printer control is installed. Only use a Bambu Studio installation you trust.

**Removal:** choose **Download removal setup**, then double-click and import the downloaded `.reg` file to remove the association. **Clear saved path** only clears this browser's preference; it does not claim to remove Windows settings. Other browser profiles or site addresses store their own path preferences, while Windows associations belong to the OS user.

## 中文

左侧导航第一项可请求打开本机 Bambu Studio，不依赖 NAS 或打印机在线。通过 **个人账户 → 本机 Bambu Studio** 修改 Windows 路径。

1. 在资源管理器找到 `bambu-studio.exe` 或 `BambuStudio.exe`，复制含盘符的完整本地路径，支持空格、中文以及“复制文件路径”附带的外层引号。
2. 点击 **保存并下载本机配置**，双击 `LayerHub-Bambu-Studio.reg` 并明确确认导入当前 Windows 用户。
3. 点击侧栏 **Bambu Studio**，确认浏览器的外部应用提示。内置浏览器拦截时，可用 Edge 或 Chrome 访问自己的 LayerHub 地址。

网页不能直接执行任意 EXE、检查本机文件和注册表，或确认桌面程序已启动，因此仅提示已请求打开。更换程序路径后需重新导入配置。本功能针对 Windows，不是跨平台安装器或桌面客户端。

路径仅存此浏览器，不传往 NAS 或云端。配置仅操作当前 Windows 用户下的固定 `layerhub-studio` 关联；启动命令只包含带引号的固定程序路径，不经过命令解释器，也不传入网页 URL 参数或文件。无需常驻程序或本机 HTTP 服务，不涉及打印机控制。请填写自己信任的 Bambu Studio 安装路径。

**移除：** 下载移除配置并双击导入，才能删除 Windows 关联。**清除浏览器路径** 只清除此浏览器偏好。其他浏览器配置文件和网站地址有各自的路径记录，而 Windows 关联属于当前系统用户。
