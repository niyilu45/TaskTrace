# TaskTrace

基于 [Vikunja](https://github.com/go-vikunja/vikunja) 二次开发的事项与项目进展跟踪工具，提供 Windows 置顶悬浮窗和完整网页界面。

**[从源码生成免安装版](https://github.com/niyilu45/TaskTrace/tree/main/dist)** · **[下载正式发布版](https://github.com/niyilu45/TaskTrace/releases)** · **[下载源码](https://github.com/niyilu45/TaskTrace/archive/refs/heads/main.zip)** · **[反馈问题](https://github.com/niyilu45/TaskTrace/issues)**

## 使用软件

1. 在 Releases 页面下载 `TaskTrace-local-windows-x64.zip`，不要选择 Source code。
2. 解压到可写目录，双击 `TaskTrace.exe`。
3. 启动后只显示悬浮窗，不会主动打开浏览器；需要完整界面时点击悬浮窗或托盘菜单中的“完整界面”。无需安装 Node.js、Go、Docker 或数据库，无需注册登录。
4. 点击悬浮窗关闭按钮或最小化按钮会隐藏到系统托盘；真正退出请在托盘菜单选择“退出 TaskTrace”。

当前测试版面向 Windows x64、本机单人使用，仅监听本机地址。多人协作需另行部署带账号登录的共享服务，不能直接通过此免登录体验包联网使用。

## 已支持

- 项目默认以紧凑展示模式浏览进展，按父事项和子任务分区；显式进入编辑模式后修改。
- 多层子任务、`to-do`、`doing`、`hold`、`done` 四种任务状态，以及已完成事项显示筛选。
- 每日进展记录、遗留问题/下一步文字记录，事项描述和每日进展支持粘贴图片。
- Windows 常驻置顶悬浮窗、搜索、托盘及完整界面入口；支持在设置或托盘菜单中启用开机启动，默认关闭；支持树形或“当前任务 / 父任务”单行路径显示，遗留事项使用粗体。
- 可配置自动保存间隔和数据保存目录。
- 启动及定时检查 GitHub Releases；可在更新设置或系统托盘手动检查，查看发布日期和更新内容，并在确认关闭后自动更新、重启。
- 完整界面及悬浮窗支持撤销按钮、Ctrl+Z，共享最近 50 组操作记录，重启后仍保留。
- 展示模式支持拖动表头右侧分隔线调整四列宽度，同一项目表格同步对齐；按项目在当前浏览器保存，可点击“恢复默认列宽”重置。
- 展示模式可按项目配置进展显示天数，以每个事项的最新进展日期为起点，显示含当天的最近 N 个自然日；支持全部、1/7/30 天及自定义。
- 展示模式可筛选最近 1/7/30 天或自定义天数内填写过每日进展的任务；命中子任务时保留其父任务路径，不显示未命中的同级分支。

遗留事项支持逐条管理、图片和跨任务拖动；悬浮窗支持层级序号、手动排序及 0～9 优先级（默认 7）。详细用法见 [免安装版说明](portable/README.md)。

## 数据与升级

默认数据位于程序目录下的 `data`，可通过 `Configure-TaskTrace.cmd` 更改。程序可自动检查并安装新版本，自动更新会保留 `data` 和 `tasktrace-settings.json`；也可退出程序后手动覆盖程序文件。

程序产物分为源码构建版和正式发布版：

```text
dist/
  Install-TaskTrace.cmd               # 检查依赖并从当前源码生成程序
  Install-TaskTrace-Engine.ps1
  TaskTrace-local/                    # 本机生成的免安装程序，不提交到 Git
Releases/
  TaskTrace-local-windows-x64.zip     # 正式发布的干净免安装包
```

Git 只跟踪 `dist` 下的安装脚本和说明，不跟踪生成的 EXE、运行数据、个人设置或日志。`Releases/` 和 `.local-build/` 也不提交到 Git；正式发布包由白名单生成，不包含测试数据、个人配置或登录会话。

每日进展支持从日期和内容表格中多选更早记录并填写更正，保留原记录及引用时的文字、图片；引用内容和引用方进展均可双向展开或收起，完整界面和悬浮窗共用，支持自动保存与撤销。

## 获取源码与构建

源码可从本仓库下载或克隆。每个版本的 Releases 页面还提供该标签对应的 Source code 下载。

下载 Source code ZIP 或使用 Git 克隆源码后，双击 `dist/Install-TaskTrace.cmd`。安装工具不依赖 `.git` 信息，也不要求安装 Git；它会从 PATH 和 Windows 常见安装位置检查 Node.js 24、pnpm 11.26、Go 1.27、Windows x64 GCC 与 .NET Framework C# 编译器，并在日志中写明实际采用的路径。如果缺少依赖，会弹出具体问题、检查范围、安装方法和日志位置；生成成功或失败时也会弹出结果，双击运行不会再因黑框关闭而看不到结果。

```powershell
cd 解压后的TaskTrace目录
dist\Install-TaskTrace.cmd
```

生成结果位于 `dist/TaskTrace-local`。这些工具只在生成程序时使用；运行成品不依赖 Node.js、pnpm、Go、GCC 或开发环境。正式发布 ZIP 仍生成在 `Releases`，构建前请先退出正在运行的 TaskTrace。详细说明见 [源码安装说明](dist/README.md)。

## 上游与许可证

TaskTrace 使用独立的 Git 提交历史，保留 Vikunja 的版权声明及原有许可证；上游项目说明见 [README.upstream.md](README.upstream.md)，许可证见 [LICENSE](LICENSE)。各目录的独立许可证同样保留。TaskTrace 的本地体验功能没有绕过上游付费功能检查。

项目列表编辑模式提供“显示已完成任务”开关，选择会自动记住。
