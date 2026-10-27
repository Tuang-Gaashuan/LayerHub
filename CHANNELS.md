# Beta and Alpha / Beta 与 Alpha

**English** · [简体中文](#简体中文)

## English

**For day-to-day team printing, use LayerHub Beta + [Bambu Studio](https://github.com/bambulab/BambuStudio).** Prepare, arrange and slice models in Bambu Studio on your computer; export the sliced plate 3MF and upload it to LayerHub Beta to confirm a printer and real spools, coordinate its queue, control supported devices and receive notifications.

| | Beta · recommended | Alpha · experimental workbench |
|---|---|---|
| Published version | [`v0.1.1-beta`](https://github.com/Tuang-Gaashuan/LayerHub/tree/v0.1.1-beta) | [`v0.4.0-alpha`](https://github.com/Tuang-Gaashuan/LayerHub/tree/v0.4.0-alpha) |
| Download | [Beta release and checksums](https://github.com/Tuang-Gaashuan/LayerHub/releases/tag/v0.1.1-beta) | [Alpha release and checksums](https://github.com/Tuang-Gaashuan/LayerHub/releases/tag/v0.4.0-alpha) |
| Development branch | [`main`](https://github.com/Tuang-Gaashuan/LayerHub/tree/main), the repository default | [`alpha`](https://github.com/Tuang-Gaashuan/LayerHub/tree/alpha), based on the last workbench-enabled release |
| Team, queues, spools, controls, email | Included; primary refinement path | Included in the Alpha baseline; relevant fixes may be adapted |
| Model workbench and geometry import | Removed | Retained, including supported STL / geometry 3MF import |
| Upload for printing | Actual already-sliced 3MF only | Sliced 3MF; supported geometry can also enter the experimental workbench |
| Model preparation and slicing | Bambu Studio on members' computers | Built-in experimental workflow; not full Bambu Studio feature parity |
| NAS slicing engine | Not required | Separate engine installation is required to use online slicing; engine binaries are not bundled |
| Development priority | Main focus: team workflows, print reliability, inventory, control, notifications and usability | Slower, gradual improvements to the built-in workbench |
| Release status | Prerelease Beta | Prerelease Alpha |

### Maintenance and version tags

- Beta and Alpha are parallel scopes in the same repository. Beta receives most feature refinement; Alpha is retained for ongoing experimental workbench development.
- Version tags such as `v0.1.1-beta` and `v0.4.0-alpha` identify fixed release snapshots. Branches move as work progresses; existing release tags and binaries are not repointed or replaced.
- Version numbers advance separately. The smaller Beta number does not mean it predates Alpha. Choose the channel by its feature scope, then read that release's notes and checksum file.
- Keep development and testing on the matching branch. Shared team/security/printer fixes can be adapted with branch-specific tests; editor and engine changes remain on Alpha. Do not merge Beta's workbench removal into Alpha wholesale.
- CI validates both development branches using the tests present on each branch. Release packages are published in batches; documentation-only updates do not require rebuilding an existing version's downloads.

### Choosing or switching

The current recommended workflow is **Bambu Studio → sliced 3MF → LayerHub Beta → team queue and printer**. Alpha remains available for users who want to experiment with the integrated workbench, within its documented import/slicing limits. Keeping Alpha does not promise support for every Bambu Studio project feature or broader printer/firmware compatibility.

Published download links above point to the named versions. A branch can contain newer, unreleased work. For separate Alpha and Beta installations, use separate data directories and ports. Never run both against one live SQLite workspace. Back up before switching and follow the [Alpha → Beta migration guide](https://github.com/Tuang-Gaashuan/LayerHub/blob/main/BETA_MIGRATION.md) for retained data, retired APIs and rollback.

## 简体中文

**日常团队打印推荐：LayerHub Beta + [Bambu Studio](https://github.com/bambulab/BambuStudio)。** 成员在电脑上用 Bambu Studio 完成模型准备、摆放与切片，导出切片盘 3MF，再上传到 LayerHub Beta，确认设备与实际料卷，进行逐机排队、受支持的设备控制及邮件通知。

| 对比项 | Beta · 推荐使用 | Alpha · 实验性内置工作台 |
|---|---|---|
| 已发布版本 | [`v0.1.1-beta`](https://github.com/Tuang-Gaashuan/LayerHub/tree/v0.1.1-beta) | [`v0.4.0-alpha`](https://github.com/Tuang-Gaashuan/LayerHub/tree/v0.4.0-alpha) |
| 下载入口 | [Beta 发布包与校验和](https://github.com/Tuang-Gaashuan/LayerHub/releases/tag/v0.1.1-beta) | [Alpha 发布包与校验和](https://github.com/Tuang-Gaashuan/LayerHub/releases/tag/v0.4.0-alpha) |
| 开发分支 | [`main`](https://github.com/Tuang-Gaashuan/LayerHub/tree/main)，仓库默认分支 | [`alpha`](https://github.com/Tuang-Gaashuan/LayerHub/tree/alpha)，基于最后一个含工作台的版本 |
| 团队、队列、耗材、控制和邮件 | 保留，作为主要完善方向 | 保留 Alpha 原有能力，按需适配相关修复 |
| 模型工作台与几何导入 | 已移除 | 保留，支持范围内可导入 STL / 几何 3MF |
| 安排打印的文件 | 仅实际已切片的 3MF | 支持切片 3MF；支持范围内的几何文件也可进入实验工作台 |
| 模型准备与切片 | 交给成员电脑上的 Bambu Studio | 使用实验性内置流程，尚未达到 Bambu Studio 全部能力 |
| NAS 切片引擎 | 不需要 | 使用在线切片需要独立安装，发布包不包含引擎程序 |
| 后续重点 | 优先完善协作流程、打印可靠性、库存、控制、通知和易用性 | 内置工作台保留，缓慢推进 |
| 发布状态 | Beta 预发布 | Alpha 预发布 |

### 维护与标签规则

- 两条路线保留在同一仓库中，主要精力用于 Beta 功能完善，Alpha 工作台继续实验性开发。
- `v0.1.1-beta`、`v0.4.0-alpha` 等版本标签对应固定发布快照；开发分支随工作推进，已发布标签和二进制包不重新指向或覆盖。
- 两条路线分别递增版本号，不能仅凭数字大小判断先后或功能多少。先按用途选择路线，再阅读该版本说明并校验下载包。
- 在对应分支开发和测试。共同的团队、安全、设备问题可分别适配与验证；编辑器、引擎功能留在 Alpha，不把 Beta 移除工作台的改动整批合入 Alpha。
- CI 分别运行各分支自己的测试。功能更新整批发布，仅修改说明文档时无需重新打包既有版本。

### 如何选择或切换

当前推荐流程为 **Bambu Studio → 已切片 3MF → LayerHub Beta → 团队队列与打印机**。需要体验内置工作台时可选择 Alpha，并以它的导入、切片支持范围为准；保留 Alpha 不代表已支持 Bambu Studio 全部工程特性或更多机型、固件。

上方下载链接对应已发布版本；开发分支可能包含尚未发布的改动。Alpha 和 Beta 同时部署时，应使用不同数据目录和端口，不能共用同一个运行中的 SQLite 工作区。切换前先备份，历史数据、停用接口与回退要求见 [Alpha → Beta 迁移说明](https://github.com/Tuang-Gaashuan/LayerHub/blob/main/BETA_MIGRATION.md)。
