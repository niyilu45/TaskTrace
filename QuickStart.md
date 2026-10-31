# TaskTrace 快速指南

TaskTrace 用于整理项目、拆分任务、记录每日进展和遗留事项。置顶悬浮窗方便随手记录，完整网页界面用于浏览、编辑、协作和备份；两者共用本机数据，无需注册登录。

本指南按当前源码界面编写，已下载的成品以所用版本的功能为准。示例使用独立演示数据，名称、路径和账号请替换为自己的实际信息。点击图片可查看大图。

- **网页 GIF**：由真实界面的分步截图合成，展示操作前后的状态。
- **悬浮窗 GIF**：由生产程序的真实控件分步示意合成，并非连续录屏；用于说明入口和操作顺序。
- **协作 GIF**：显眼标注的独立内存配置示意，没有实际修改 Windows 共享权限或执行局域网同步，不能作为真实同步成功的证明。

**第一次使用：下载 → 启动 → 创建项目 → 添加任务 → 保存进展 → 设置备份。**

## 目录

- [1. 下载与启动](#1-下载与启动)
- [2. 网页：建立项目并浏览进展](#2-网页建立项目并浏览进展)
- [3. 网页：记录每日进展与遗留事项](#3-网页记录每日进展与遗留事项)
- [4. 悬浮窗：浏览、记录与快捷操作](#4-悬浮窗浏览记录与快捷操作)
- [5. 团队协作：分享与接收](#5-团队协作分享与接收)
- [6. 周期回顾与 Markdown 导出](#6-周期回顾与-markdown-导出)
- [7. 备份、恢复与升级](#7-备份恢复与升级)
- [8. 常见问题](#8-常见问题)

## 1. 下载与启动

### 使用免安装包

1. 打开 [GitHub Releases](https://github.com/niyilu45/TaskTrace/releases/latest)，下载 `TaskTrace-local-windows-x64.zip`。`Source code` 是源码，需要自行构建。
2. 完整解压到可写目录，例如 `D:\Apps\TaskTrace`，不要直接在压缩包中运行。
3. 双击 `TaskTrace.exe`。成品运行不需要安装 Node.js、pnpm、Go 或数据库。
4. 启动后显示悬浮窗。点击“完整界面”，或右击托盘图标选择同名菜单，打开浏览器中的完整界面。

[![启动、完整界面与托盘退出流程](docs/images/quickstart/01-startup-flow.svg)](docs/images/quickstart/01-startup-flow.svg)

关闭或最小化悬浮窗只是隐藏到系统托盘，双击托盘图标可恢复。真正退出请选择托盘菜单“退出 TaskTrace”。开机启动默认关闭，可在托盘菜单中启用。

### 从源码安装

下载并解压 [仓库源码](https://github.com/niyilu45/TaskTrace/archive/refs/heads/main.zip)，双击 `dist/Install-TaskTrace.cmd`，粘贴或选择安装目录。安装器先检查运行中的程序及构建依赖，再生成程序；缺少依赖时按提示补齐。

选择已有程序目录时，只更新程序文件并保留配置和数据。若提示强制关闭，先保存编辑内容再继续。安装完成后可选择立即打开软件。依赖、系统代理与日志详见 [源码安装说明](dist/README.md)。

## 2. 网页：建立项目并浏览进展

### 新建第一个项目

从项目列表进入创建项目，填写名称并保存，再打开项目。

[![操作演示：创建项目并进入项目](docs/images/quickstart/gifs/w01-create-project.gif)](docs/images/quickstart/gifs/w01-create-project.gif)

<details>
<summary>查看创建项目界面图解</summary>

[![创建项目的名称与保存入口](docs/images/quickstart/maps/18-new-project-map.svg)](docs/images/quickstart/maps/18-new-project-map.svg)

</details>

| 内容 | 用途 | 示例 |
| --- | --- | --- |
| 项目 | 一张总表，容纳多个任务 | 产品验证 |
| 任务 / 子任务 | 独立的描述、每日进展和完成状态，任务层级最多 5 级 | 测试准备 → 准备测试环境 |
| 遗留事项 | 隶属某任务、跨日期共享的待解决清单，可含备注和图片 | 等待补充测试账号 |

### 添加任务与子任务

进入项目默认是展示模式。点击“进入编辑模式”，在“添加任务…”中填写名称并添加；点击任务名打开详情，在子任务区域继续拆分。完成后点击“返回展示模式”。

[![操作演示：进入编辑模式并添加任务](docs/images/quickstart/gifs/w02-create-task.gif)](docs/images/quickstart/gifs/w02-create-task.gif)

<details>
<summary>查看项目任务创建界面图解</summary>

[![项目编辑模式中的添加任务入口](docs/images/quickstart/maps/17-create-collaborative-task-map.svg)](docs/images/quickstart/maps/17-create-collaborative-task-map.svg)

</details>

### 按总表浏览进展

展开任务及其子任务，可同时浏览名称、描述、遗留事项与每日进展。任务自身进展位于子任务前，进展按记录日期倒序显示；可拖动表头分隔线调整列宽。

[![操作演示：浏览层级任务与每日进展](docs/images/quickstart/gifs/w03-browse-progress.gif)](docs/images/quickstart/gifs/w03-browse-progress.gif)

<details>
<summary>查看项目展示模式界面图解</summary>

[![项目展示模式的进展表格与操作入口](docs/images/quickstart/maps/02-project-overview-map.svg)](docs/images/quickstart/maps/02-project-overview-map.svg)

</details>

### 筛选任务与调整归属

“显示选项”可多选待办、进行中、暂停、已完成。“人员”默认只选自己，查看协作内容时按需勾选其他成员。

[![操作演示：筛选人员、状态与近期进展](docs/images/quickstart/gifs/w04-filter-tasks.gif)](docs/images/quickstart/gifs/w04-filter-tasks.gif)

- **最近进展**：筛选多少天内填写过进展的任务，保留匹配任务的父路径，未匹配的同级分支不显示。
- **进展范围**：限制每条任务展示的历史，从该任务最新进展日期往前计算；点击已隐藏的较早进展，可展开该任务的全部进展。
- **调整任务归属**：到左侧“概览”首页，按住任务名拖到另一任务或项目虚线框，拖动期间可用滚轮滚动。移动会携带子任务和遗留事项；悬浮窗中会提示到网页概览操作。

## 3. 网页：记录每日进展与遗留事项

### 每日进展：选日期、填内容、保存

打开任务详情，在“记录每日进展”选择日期，在“我的进展”中填写文字或粘贴图文，最后点击“保存进展”。日期下的蓝点表示当天已有记录，切换日期即可补记或编辑历史内容。

[![操作演示：选择日期并填写每日进展](docs/images/quickstart/gifs/w05-daily-progress.gif)](docs/images/quickstart/gifs/w05-daily-progress.gif)

<details>
<summary>查看每日进展与日期选择界面图解</summary>

[![每日进展图文编辑与保存](docs/images/quickstart/maps/03-daily-progress-map.svg)](docs/images/quickstart/maps/03-daily-progress-map.svg)

[![日期选择中的月份与进展标记](docs/images/quickstart/maps/19-progress-calendar-map.svg)](docs/images/quickstart/maps/19-progress-calendar-map.svg)

</details>

**每日进展和遗留事项的自动保存只缓存 `.cache` 草稿，不等于正式提交，也不会把草稿同步给协作成员。请以手动保存成功为准。**

图片直接嵌入正文，可选中后按退格或 Delete 删除。当天有其他成员进展时，会并列显示相应编辑框，具备写权限时可以修改；评论记录标明实际修改人。

需要更正旧记录时，在新日期点击“引用历史进展”，勾选记录、确认，再填写更正并保存。引用保留当时内容，原记录不被替换。

### 遗留事项：跨日期的独立清单

点击“新增遗留事项”，在标题同行选择优先级，填写后点击“添加遗留事项”。“添加遗留事项备注”可展开图文备注；已有条目点击“编辑”，修改后点击“保存修改”。

[![操作演示：新增遗留事项及图文备注](docs/images/quickstart/gifs/w06-outstanding.gif)](docs/images/quickstart/gifs/w06-outstanding.gif)

<details>
<summary>查看遗留事项界面图解</summary>

[![遗留事项内容、优先级、备注与保存](docs/images/quickstart/maps/04-outstanding-map.svg)](docs/images/quickstart/maps/04-outstanding-map.svg)

</details>

勾选完成后，网页编辑列表会在 5 秒后移入已完成列表；点击“显示已完成的遗留事项”可查看。遗留清单在所有日期共用，备注只在编辑窗口查看，不在悬浮窗清单中展开。

## 4. 悬浮窗：浏览、记录与快捷操作

以下 GIF 为真实生产控件的分步操作示意，便于查找功能，并非连续录屏。新增、批量导入和提醒示例停留在填写或预览阶段，未正式提交。各组“界面图解”可展开查看。

### 浏览、搜索、单行与简洁模式

选择项目后按任务树浏览。查找事项及状态、优先级筛选会影响可见条目；“单行显示”将末级内容与上级路径合并，符合规则的任务才独立显示。

[![悬浮窗分步示意：完整、简洁、单行显示与选择](docs/images/quickstart/gifs/native-window.gif)](docs/images/quickstart/gifs/native-window.gif)

<details>
<summary>查看完整、简洁、单行和选择界面图解</summary>

[![完整悬浮窗的项目与任务树](docs/images/quickstart/maps/n-window-01-map.svg)](docs/images/quickstart/maps/n-window-01-map.svg)

[![简洁模式返回完整与最小化](docs/images/quickstart/maps/n-window-03-map.svg)](docs/images/quickstart/maps/n-window-03-map.svg)

[![单行显示中的任务与上级路径](docs/images/quickstart/maps/n-window-05-map.svg)](docs/images/quickstart/maps/n-window-05-map.svg)

[![查找后全选当前可见条目](docs/images/quickstart/maps/n-window-06-map.svg)](docs/images/quickstart/maps/n-window-06-map.svg)

</details>

简洁模式选中后显示“完整”和“最小化”。“完整”恢复完整悬浮窗；“完整界面”打开浏览器。两种悬浮窗共用条目与筛选逻辑。

右击托盘可“重置悬浮窗位置”；勾选“边缘隐藏”后贴边收起，鼠标靠近原位置恢复，完整与简洁模式都支持。悬浮主窗关闭或最小化只隐藏到托盘。

### 右键、多选、完成度与排序

`Ctrl+单击` 多选、`Shift+单击` 连选、`Ctrl+A` 全选当前筛选后可见条目；右击可批量设置优先级、状态、背景色和完成度，也可全部展开或收起。

[![悬浮窗分步示意：右键菜单、完成度、排序与设置](docs/images/quickstart/gifs/native-actions.gif)](docs/images/quickstart/gifs/native-actions.gif)

<details>
<summary>查看右键菜单、状态、颜色、完成度、排序、设置与优先级图解</summary>

[![右键菜单与批量操作入口](docs/images/quickstart/maps/n-actions-01-map.svg)](docs/images/quickstart/maps/n-actions-01-map.svg)

[![右键直接修改待办、进行中、暂停和已完成](docs/images/quickstart/maps/n-actions-02-map.svg)](docs/images/quickstart/maps/n-actions-02-map.svg)

[![设置仅在本机生效的背景色](docs/images/quickstart/maps/n-actions-03-map.svg)](docs/images/quickstart/maps/n-actions-03-map.svg)

[![按百分比设置填写完成度](docs/images/quickstart/maps/n-actions-04-map.svg)](docs/images/quickstart/maps/n-actions-04-map.svg)

[![优先级与完成度多级排序](docs/images/quickstart/maps/n-actions-05-map.svg)](docs/images/quickstart/maps/n-actions-05-map.svg)

[![显示优先级、默认值与完成后的外观设置](docs/images/quickstart/maps/n-actions-06-map.svg)](docs/images/quickstart/maps/n-actions-06-map.svg)

[![任务和遗留事项的个人优先级](docs/images/quickstart/maps/n-actions-07-map.svg)](docs/images/quickstart/maps/n-actions-07-map.svg)

</details>

- **优先级**为 0～9，数字越小越优先，默认新增为 7；也可点击条目前的优先级文字修改。
- **优先级和背景色只影响本机**，不会改变协作成员的设置。子任务背景色可继承父任务，也可单独指定。
- **完成度**默认 0%，以 10% 为步进；勾选“显示完成度”后如“待办-70%”。勾选完成覆盖显示为 100%，取消后恢复原填写值。
- **排序方式**可组合优先级和完成度的升序、降序；树状模式只调整同级顺序。
- **设置**可调整优先级筛选、默认新增优先级、完成后的灰色/删除线及刚切换完成后的隐藏延迟。`F5` 刷新；列表中 `Ctrl+Z` 撤销操作，编辑框中撤销输入。

### 新事项、子任务与批量导入

| 目标 | 入口 |
| --- | --- |
| 项目中的顶层任务 | 选择项目 → 新事项 |
| 选中任务下的子任务 | 选择父任务 → 新任务 |
| 任务的遗留清单 | 选择任务 → 新遗留 |
| 多条文字转成任务或遗留事项 | 对应新增窗口 → 批量导入… |

[![悬浮窗分步示意：新建任务、子任务与批量导入](docs/images/quickstart/gifs/native-create.gif)](docs/images/quickstart/gifs/native-create.gif)

<details>
<summary>查看新事项、子任务与批量导入界面图解</summary>

[![新增事项名称与优先级](docs/images/quickstart/maps/n-create-01-map.svg)](docs/images/quickstart/maps/n-create-01-map.svg)

[![向选中任务添加子任务](docs/images/quickstart/maps/n-create-02-map.svg)](docs/images/quickstart/maps/n-create-02-map.svg)

[![批量导入每行一条的文字](docs/images/quickstart/maps/n-create-03-map.svg)](docs/images/quickstart/maps/n-create-03-map.svg)

[![修改导入预览并确认](docs/images/quickstart/maps/n-create-04-map.svg)](docs/images/quickstart/maps/n-create-04-map.svg)

</details>

批量导入自动过滤空行、分隔符及纯标点行，去掉开头序号和首尾空格。确认前可在预览中修改文字，选中整行按 Delete 移除；检查无误再“确认导入”。

### 每日进展、任务名与描述

双击任务或点击“记录进展”，选择日期、填写图文，点击“保存当天进展”，也可 `Ctrl+Enter` 保存。选中遗留事项后点“记录进展”，编辑的是其所属任务的进展。

[![悬浮窗分步示意：日期、任务描述与历史引用](docs/images/quickstart/gifs/native-progress.gif)](docs/images/quickstart/gifs/native-progress.gif)

<details>
<summary>查看每日进展、历史日期、任务描述与引用界面图解</summary>

[![任务编辑窗中的任务名、日期、进展及保存](docs/images/quickstart/maps/n-progress-01-map.svg)](docs/images/quickstart/maps/n-progress-01-map.svg)

[![按蓝点查找已记录的日期](docs/images/quickstart/maps/n-progress-02-map.svg)](docs/images/quickstart/maps/n-progress-02-map.svg)

[![选择已有进展日期并编辑](docs/images/quickstart/maps/n-progress-03-map.svg)](docs/images/quickstart/maps/n-progress-03-map.svg)

[![展开任务描述后保存或取消](docs/images/quickstart/maps/n-progress-04-map.svg)](docs/images/quickstart/maps/n-progress-04-map.svg)

[![在历史记录表格中多选引用](docs/images/quickstart/maps/n-progress-05-map.svg)](docs/images/quickstart/maps/n-progress-05-map.svg)

</details>

任务名修改后点击“保存任务名”；描述通过“查看/编辑任务描述”展开，单独保存或取消。日期蓝点提示已有记录，“回到今天”可快速返回当前日期；历史日期直接载入该日进展，协作成员内容按实际记录显示。

“引用历史进展”支持多选，确认后在当天填写说明再保存。原进展保留，引用内容可展开/收起。**进展自动缓存不是提交**；关闭有修改的窗口可保存、不保存或取消，选择不保存会丢弃草稿。

### 历史完整信息、图片与导出

“查看历史进展”列出各日期记录。点击“完整信息”保留原图文排版；点击带下划线的图片数量，只查看该记录的图片。

[![悬浮窗分步示意：历史列表、完整信息与大图](docs/images/quickstart/gifs/native-history.gif)](docs/images/quickstart/gifs/native-history.gif)

<details>
<summary>查看历史列表与完整信息界面图解</summary>

[![历史进展记录的完整信息和图片入口](docs/images/quickstart/maps/n-history-01-map.svg)](docs/images/quickstart/maps/n-history-01-map.svg)

[![按原文字图片排版浏览完整记录](docs/images/quickstart/maps/n-history-03-map.svg)](docs/images/quickstart/maps/n-history-03-map.svg)

[![大图窗口的自适应、缩放与任务栏入口](docs/images/quickstart/maps/n-history-04-map.svg)](docs/images/quickstart/maps/n-history-04-map.svg)

</details>

完整信息打开时定位所选日期，滚动到边界后继续滚动会加载其他日期。文字、图片可复制；双击图片打开大图，自动适配屏幕，可滚轮缩放。

任务编辑窗的导出入口可预览或保存 Markdown，包含已保存内容和可读取的子任务，图片内嵌。不同任务/遗留事项支持多窗编辑，可拖动和最小化；再次打开同一条目会唤回已有窗口。

### 遗留事项与图文复制

双击遗留事项可修改内容、个人优先级、背景色和图文备注。“保存修改”正式提交后仍停留当前条目；“编辑所属任务”打开所属任务编辑窗。

[![悬浮窗分步示意：遗留事项备注、图片和撤销](docs/images/quickstart/gifs/native-outstanding.gif)](docs/images/quickstart/gifs/native-outstanding.gif)

<details>
<summary>查看遗留事项内容、图文备注与保存界面图解</summary>

[![遗留事项内容、优先级与本机背景色](docs/images/quickstart/maps/n-outstanding-01-map.svg)](docs/images/quickstart/maps/n-outstanding-01-map.svg)

[![在备注中插入文字和图片](docs/images/quickstart/maps/n-outstanding-02-map.svg)](docs/images/quickstart/maps/n-outstanding-02-map.svg)

[![撤销图文修改后保存当前条目](docs/images/quickstart/maps/n-outstanding-04-map.svg)](docs/images/quickstart/maps/n-outstanding-04-map.svg)

</details>

每日进展、备注和快捷备忘支持粘贴混合图文，选中后 `Ctrl+C`，可粘贴到其他编辑窗或网页编辑区。外部图文粘贴保留排版、缩进和换行，去掉外部字体、颜色及背景样式。`Ctrl+Z` 撤销输入；图片可退格/Delete删除，双击放大。

备注不会在悬浮窗任务清单展开；遗留事项和进展的未保存修改只缓存，正式保存后才写入任务。

### 单次与周期提醒

右击任务或遗留事项选择“设置提醒”，编辑窗也有同名入口。有提醒的条目前可点击“提醒”重新设置。

[![悬浮窗分步示意：单次、按周重复与日期范围](docs/images/quickstart/gifs/native-reminder.gif)](docs/images/quickstart/gifs/native-reminder.gif)

<details>
<summary>查看提醒日期、每周多选、范围与保存界面图解</summary>

[![设置具体提醒日期和时刻](docs/images/quickstart/maps/n-reminder-01-map.svg)](docs/images/quickstart/maps/n-reminder-01-map.svg)

[![每周多选星期进行提醒](docs/images/quickstart/maps/n-reminder-02-map.svg)](docs/images/quickstart/maps/n-reminder-02-map.svg)

[![限制周期开始和结束日期](docs/images/quickstart/maps/n-reminder-03-map.svg)](docs/images/quickstart/maps/n-reminder-03-map.svg)

[![确认提醒清单后保存](docs/images/quickstart/maps/n-reminder-04-map.svg)](docs/images/quickstart/maps/n-reminder-04-map.svg)

</details>

周期提醒可多选星期并限定日期范围，起止当天包含在内。检查窗口清单并保存后生效；示意中取消不会提交提醒。到时弹窗可“关闭”本次，或“延迟”后再提醒；关闭本次不影响后续周期日期。网页与悬浮窗使用相同提醒数据。

### 便捷闹钟

右击托盘 → “便捷闹钟”，勾选开始倒计时，多个闹钟可以同时运行；取消勾选停止并恢复预设时间，不弹完成提示。

[![悬浮窗分步示意：预设、并行、任意时长与完成提示](docs/images/quickstart/gifs/native-alarm.gif)](docs/images/quickstart/gifs/native-alarm.gif)

<details>
<summary>查看闹钟列表、编辑时长与任意闹钟界面图解</summary>

[![便捷闹钟列表与倒计时入口](docs/images/quickstart/maps/n-alarm-01-map.svg)](docs/images/quickstart/maps/n-alarm-01-map.svg)

[![分别编辑五个闹钟的预设时间](docs/images/quickstart/maps/n-alarm-03-map.svg)](docs/images/quickstart/maps/n-alarm-03-map.svg)

[![填写任意闹钟的本次时长](docs/images/quickstart/maps/n-alarm-04-map.svg)](docs/images/quickstart/maps/n-alarm-04-map.svg)

[![闹钟完成后的确认提示](docs/images/quickstart/maps/n-alarm-05-map.svg)](docs/images/quickstart/maps/n-alarm-05-map.svg)

</details>

五个预设闹钟可从“编辑闹钟”修改时长；“任意闹钟”每次填写本次时间。到时自动取消勾选并恢复预设，中央弹窗点击确认关闭。闹钟示意使用演示时钟；它与绑定任务日期的“事项提醒”相互独立。

### 快捷备忘

右击托盘 → “快捷备忘”，随手记文字和图片；重复打开会唤回同一个窗口，关闭后重新打开保留内容。

[![悬浮窗分步示意：快捷备忘图文与自动保存](docs/images/quickstart/gifs/native-memo.gif)](docs/images/quickstart/gifs/native-memo.gif)

<details>
<summary>查看快捷备忘内容与保存状态界面图解</summary>

[![快捷备忘图文编辑与立即保存](docs/images/quickstart/maps/n-memo-01-map.svg)](docs/images/quickstart/maps/n-memo-01-map.svg)

[![快捷备忘保存状态与图片数量](docs/images/quickstart/maps/n-memo-03-map.svg)](docs/images/quickstart/maps/n-memo-03-map.svg)

</details>

**快捷备忘会自动正式保存到本机，区别于每日进展和遗留事项的草稿缓存。** 可点击“立即保存”，底部查看保存状态和图片数量；失败时保留窗口供重试。

备忘保存在实际数据目录的 `quick-memo` 下，纳入个人数据备份；保留原数据目录的 install 更新不会清空备忘，不加入团队任务同步。

## 5. 团队协作：分享与接收

每人运行自己的 TaskTrace，通过局域网 Windows 共享文件夹交换协作数据，不需要开放网页服务。

**本节协作 GIF 是“协作配置示意（不改 Windows 共享权限）”。示例成员、链接、权限与导入结果仅在隔离内存中模拟，没有执行真实共享授权或网络同步。实际使用请按下列步骤配置并核对结果。**

### 发起方：选成员、分享任务、复制链接

1. 在项目中创建任务，或打开已有任务。
2. 左侧“团队 → 局域网协作 → 所有协作人员”，点击“添加协作成员”，按姓名、工号或邮箱搜索并选择，等待结果。
3. 本机 `teamData` 未共享时按提示确认创建共享，确需管理员权限时再确认临时授权。
4. 回到任务，展开默认收起的“团队协作”，选择成员，点击“共享当前任务”。
5. 在“任务链接”旁“复制链接”，提供给指定接收者。可“分配给用户”选择受理人，再从“编辑权限”核对权限。

[![协作配置示意：选择成员、分享与权限卡片，不修改Windows权限](docs/images/quickstart/gifs/w07-team-sharing.gif)](docs/images/quickstart/gifs/w07-team-sharing.gif)

<details>
<summary>查看团队人员和任务共享界面图解</summary>

[![所有协作人员与最高权限管理](docs/images/quickstart/maps/15-team-members-map.svg)](docs/images/quickstart/maps/15-team-members-map.svg)

[![根据姓名工号邮箱选择协作人员](docs/images/quickstart/maps/21-team-member-search-map.svg)](docs/images/quickstart/maps/21-team-member-search-map.svg)

[![任务卡片的团队协作区域](docs/images/quickstart/maps/05-team-sharing-map.svg)](docs/images/quickstart/maps/05-team-sharing-map.svg)

[![已共享任务的链接与同步入口](docs/images/quickstart/maps/23-team-link-map.svg)](docs/images/quickstart/maps/23-team-link-map.svg)

[![逐人勾选任务读写权限](docs/images/quickstart/maps/22-team-permissions-map.svg)](docs/images/quickstart/maps/22-team-permissions-map.svg)

</details>

只有自己不构成多人协作。分享包含当前任务、子任务、遗留事项与进展，上级及未分享同级任务不在范围内。

### 接收方：导入到已有项目或新项目

1. 先在文件资源管理器中确认能访问分享者的实际 `teamData` 网络共享路径。
2. 网页左侧“概览 → 导入共享任务”，粘贴完整 `tasktrace-team://` 链接。
3. “保存到项目”选择已有项目，或“＋ 新建项目…”并填名称。
4. 必要时补“共享目录地址”，例如 `\\DEMO-PC\teamData`；原地址含 `imports` 子目录时必须保留完整路径。
5. 点击“导入任务”，真实导入后到任务“团队协作 → 协作连接信息”核对标识和实际目录。

[![协作配置示意：导入共享链接并选择项目，不执行真实网络导入](docs/images/quickstart/gifs/w08-import-shared-task.gif)](docs/images/quickstart/gifs/w08-import-shared-task.gif)

<details>
<summary>查看共享任务导入与新项目选择界面图解</summary>

[![导入共享链接并选择已有项目](docs/images/quickstart/maps/06-import-shared-task-map.svg)](docs/images/quickstart/maps/06-import-shared-task-map.svg)

[![导入共享任务时新建项目](docs/images/quickstart/maps/16-import-new-project-map.svg)](docs/images/quickstart/maps/16-import-new-project-map.svg)

</details>

本地可重新安排接收任务的归属，不改变其他成员的结构。旧子任务链接不会自动扩大到后来分享的父任务，接收者需要导入新的父任务链接。

### 继续记录、权限与同步

- 新增子任务或遗留事项继承协作成员与权限，可再按权限调整。先正式保存内容，草稿不向成员同步。
- 团队页管理 `teamData` **最高权限**；任务“编辑权限”进一步限制任务和遗留事项。所属人、受理人具有读写权限，其他成员按配置执行；保存还需共享文件夹实际允许写入。
- 个人优先级仅本机保存，不受共享任务只读限制，但仍需本地项目编辑权限。
- 托盘或项目页“进入编辑模式”左侧的“立即同步”，同步已保存内容；双方核对相同共享范围、协作标识及实际同步目录。能打开权限页不代表同步成功。
- 成员更新、冲突在顶部铃铛查看；查看更新定位内容并标为已读，清除后可从历史通知查看。冲突界面逐项对比与处理。
- 网络中断时保留本地修改，恢复连接后同步。不要手改 `teamData` 同步文件；发起方可用“更新后通知其他协作成员”控制通知。

## 6. 周期回顾与 Markdown 导出

### 按项目与日期汇总变化

网页左侧“概览 → 周期回顾”，默认全部项目、过去 7 天。可改为指定日期范围，点击“查看（新标签页）”后，在报告导航按项目或任务定位。

[![操作演示：选择周期范围并查看回顾报告](docs/images/quickstart/gifs/w09-period-review.gif)](docs/images/quickstart/gifs/w09-period-review.gif)

<details>
<summary>查看周期回顾范围设置界面图解</summary>

[![周期回顾的项目和时间范围](docs/images/quickstart/maps/07-period-review-map.svg)](docs/images/quickstart/maps/07-period-review-map.svg)

[![周期回顾报告的目录、表格与导出入口](docs/images/quickstart/maps/20-period-review-report-map.svg)](docs/images/quickstart/maps/20-period-review-report-map.svg)

</details>

报告按展示模式排版，只列出周期内每日进展或遗留事项有变化的任务。遗留事项展示当前内容，标记新增、完成或改动；已删除内容和周期外已完成的遗留不展示。报告内可导出当前展示的 Markdown。

### 导出单个任务

网页任务详情卡片右侧“管理 → 导出 Markdown”，或悬浮窗任务编辑窗的导出入口，可先预览，再选择保存位置。文件包含已保存的描述、每日进展、遗留事项及可读取的子任务。

图片内嵌到 Markdown 文件，部分外部查看器不支持，可在程序预览。成功后网页通知显示保存位置及打开所在文件夹的链接；写入失败会通知，取消保存不通知。

## 7. 备份、恢复与升级

### 每天定时备份，也可立即备份

网页“设置 → 数据保护与恢复”启用定时备份，设置路径、每天执行时间、保留天数和最低保留份数，点击“保存备份设置”。程序需在计划时间运行，也可随时“立即备份”；数据未改变时不新增备份。

[![定时备份的路径、每日时刻与保留策略](docs/images/quickstart/maps/08-backup-settings-map.svg)](docs/images/quickstart/maps/08-backup-settings-map.svg)

备份包括配置的个人与团队数据。过期备份会自动清理，但保留最低份数；建议将备份放在程序目录外。

### 从备份或复制目录恢复

- **备份**：点击“查看可恢复备份”，核对日期、备注和路径，再“恢复此备份”。可修改备注；手动删除是永久删除，不受最低保留份数保护。
- **复制目录**：在“检测数据”填写 `data` 路径；配套 `teamData` 不在旁边时，另填团队数据路径后检测。单独 `teamData` 不含个人数据库，需要配套 `data`。

[![检测数据与恢复已有数据](docs/images/quickstart/maps/09-data-recovery-map.svg)](docs/images/quickstart/maps/09-data-recovery-map.svg)

恢复和导入复制到新的受保护目录，当前和源数据不直接覆盖。按提示退出并重新启动后生效。

### 保存位置与安全升级

| 位置 | 内容 |
| --- | --- |
| `data` | 个人数据库、图片、本地协作任务副本、身份与本地设置；快捷备忘在其中的 `quick-memo` |
| `teamData` | 交换协作任务的团队仓库 |
| `tasktrace-settings.json` | 本机数据目录和备份配置 |
| `.cache` | 每日进展与遗留事项的未提交草稿 |
| `backups` | 默认备份目录，可在设置中修改 |

以上目录可因配置而变化。`Configure-TaskTrace.cmd` 可改数据目录，单改路径不搬移文件；迁移优先使用“数据保护与恢复”。

网页“设置 → 更新设置”或托盘“检查更新”可查看版本差异并更新。默认启动检查，此后每 60 分钟检查，可修改间隔，联网使用 Windows 系统代理。

[![自动更新间隔、当前版本与检查更新](docs/images/quickstart/maps/10-update-settings-map.svg)](docs/images/quickstart/maps/10-update-settings-map.svg)

手动升级先保存内容并退出程序，再用 Release ZIP 或源码安装器生成的 `dist/TaskTrace-program-files.zip` 覆盖程序文件。**保留 `data`、`teamData`、配置、草稿与备份，不要将另一台电脑运行过的整个程序目录覆盖过来。**

## 8. 常见问题

| 问题 | 先检查 |
| --- | --- |
| 提示已有程序运行 | 查看托盘，退出原实例；隐藏窗口不等于退出 |
| 悬浮窗条目少 | 同一项目、清空搜索、优先级全部、状态全选，再检查树收起及单行模式 |
| 看不到其他成员的内容 | 人员筛选、对方正式保存；双方立即同步，核对共享范围、标识、实际目录（含 `imports`）与最近错误 |
| 能看到文件夹但保存失败 | 核对共享/文件权限和任务写权限；需要时按提示临时授权 |
| 关闭提示未保存 | 进展/遗留只有缓存未提交时需手动保存；快捷备忘则看底部自动保存结果 |
| 安装失败 | 按提示补齐依赖，查看 `dist/install.log` 和 [源码安装说明](dist/README.md) |
| 检查更新失败 | 检查网络及系统代理，或 [手动下载 Release](https://github.com/niyilu45/TaskTrace/releases) |

反馈请提供版本、步骤及错误详情，通过 [GitHub Issues](https://github.com/niyilu45/TaskTrace/issues) 提交。请勿上传真实任务数据、团队链接或含凭据的配置与会话文件。
