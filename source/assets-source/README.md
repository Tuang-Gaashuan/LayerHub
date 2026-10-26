# 打印机示意插画源码

这些是本项目制作的界面识别插画，SVG 是源文件，web/assets/printers 中同时保留 SVG 与 PNG。它们不是官方商品照片、可旋转三维模型或工程 CAD，比例和结构经过简化。原创代码和插画随本项目 AGPL-3.0-only 提供；产品名称及商标归各自权利人，本项目与厂商不存在官方隶属关系。

`bambu/` 和 `other-brands/` 保留生成脚本，移除了开发机专有路径。需要重新生成时在相应目录安装其 package.json 中的 sharp，再运行 `node generate.mjs`。工具仅为重绘素材所需，不是应用运行依赖。输出需经视觉核对后再复制到 web/assets/printers。
