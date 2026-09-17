# TaskTrace

基于 [Vikunja](https://github.com/go-vikunja/vikunja) 二次开发的事项与项目进展跟踪工具，提供 Windows 置顶悬浮窗和完整网页界面。

**[下载 Windows 免安装测试版](https://github.com/niyilu45/TaskTrace/releases)** · **[下载源码](https://github.com/niyilu45/TaskTrace/archive/refs/heads/main.zip)** · **[反馈问题](https://github.com/niyilu45/TaskTrace/issues)**

## 使用软件

1. 在 Releases 页面下载 `TaskTrace-local-windows-x64.zip`，不要选择 Source code。
2. 解压到可写目录，双击 `TaskTrace.exe`。
3. 同时打开悬浮窗和完整界面，无需安装 Node.js、Go、Docker 或数据库，无需注册登录。
4. 关闭浏览器后悬浮窗继续运行；点击悬浮窗关闭按钮或最小化按钮会隐藏到系统托盘，完整界面仍可使用；真正退出请在托盘菜单选择“退出 TaskTrace”。

当前测试版面向 Windows x64、本机单人使用，仅监听本机地址。多人协作需另行部署带账号登录的共享服务，不能直接通过此免登录体验包联网使用。

## 已支持

- 项目默认以紧凑展示模式浏览进展，按父事项和子任务分区；显式进入编辑模式后修改。
- 多层子任务、完成状态及已完成事项显示筛选。
- 每日进展记录、遗留问题/下一步文字记录，事项描述和每日进展支持粘贴图片。
- Windows 常驻置顶悬浮窗、搜索、分页、收起、托盘及完整界面入口。
- 可配置自动保存间隔和数据保存目录。
- 完整界面及悬浮窗支持撤销按钮、Ctrl+Z，共享最近 50 组操作记录，重启后仍保留。
- 展示模式支持拖动表头右侧分隔线调整四列宽度，同一项目表格同步对齐；按项目在当前浏览器保存，可点击“恢复默认列宽”重置。
- 展示模式可按项目配置进展显示天数，以每个事项的最新进展日期为起点，显示含当天的最近 N 个自然日；支持全部、1/7/30 天及自定义。

遗留事项支持逐条管理、图片和跨任务拖动；悬浮窗支持层级序号、手动排序及 0～9 优先级（默认 9）。详细用法见 [免安装版说明](portable/README.md)。

## 数据与升级

默认数据位于程序目录下的 `data`，可通过 `Configure-TaskTrace.cmd` 更改。升级前退出程序，保留 `data` 和 `tasktrace-settings.json`，再覆盖程序文件。

本地交付目录统一为：

```text
Releases/
  TaskTrace-local/                    # 可直接测试的程序及本地数据
  TaskTrace-local-windows-x64.zip     # 可公开发布的干净免安装包
```

`Releases/` 和 `.local-build/` 不提交到 Git。发布包由白名单生成，不包含测试数据、个人配置或登录会话；不要直接压缩使用中的程序目录上传。

## 获取源码与构建

源码可从本仓库下载或克隆。每个版本的 Releases 页面还提供该标签对应的 Source code 下载。

Windows 构建需要 Git、Node.js 24 或更新版本、项目指定的 pnpm、Go（版本见 go.mod）、x64 GCC，以及 .NET Framework C# 编译器。

```powershell
git clone git@github.com:niyilu45/TaskTrace.git
cd TaskTrace
powershell.exe -NoProfile -ExecutionPolicy Bypass -File portable/Build-Local.ps1
```

构建结果位于 `Releases`，运行成品不依赖上述开发工具。构建前先退出该目录中运行的 TaskTrace。

## 上游与许可证

TaskTrace 使用独立的 Git 提交历史，保留 Vikunja 的版权声明及原有许可证；上游项目说明见 [README.upstream.md](README.upstream.md)，许可证见 [LICENSE](LICENSE)。各目录的独立许可证同样保留。TaskTrace 的本地体验功能没有绕过上游付费功能检查。

项目列表编辑模式提供“显示已完成任务”开关，选择会自动记住。
