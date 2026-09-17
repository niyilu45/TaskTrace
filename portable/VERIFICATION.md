# 本机体验版验证记录

日期：2026-09-16

已生成 dist/TaskTrace-local-windows-x64.zip，面向 Windows x64。

已实测：
- PATH 仅保留 Windows 系统目录，无 Node.js、Go、pnpm、GCC 时正常启动。
- 在带空格和中文的路径中运行正常。
- 内嵌网页首页及 JavaScript 资源返回成功。
- 注册、登录、创建项目与任务、追加评论成功。
- 同一事项上传两张 PNG 图片成功。
- 服务退出后重启，登录令牌、任务、评论与图片均保留。
- 下载图片的 SHA256 与上传原文件一致。
- 默认端口被占用后自动改用 3457。
- 重复启动相同数据目录被安全阻止。
- 强制结束测试启动器后，其后台服务被 Windows Job Object 自动停止。
- ZIP 使用明确文件白名单，不包含测试账号、数据库、附件、密钥或构建工具。

## 免登录更新验证

- 全新数据目录启动时自动建立本地工作区，无需注册或输入账号密码。
- 最终打包成品在无 Node.js 的 PATH 下成功启动。
- 通过仓库 mage test:e2e 运行 Edge/Playwright：直接进入工作区、刷新页面、过期会话续期、实际新增事项全部通过。
- 旧单账号测试数据库自动沿用原用户，原任务和两张图片可读取；不修改原密码。
- 认证相关 26 项前端回归测试通过；修改的前端文件 ESLint 通过。
- 前后端构建成功。全量前端类型检查未通过，仓库仍有大量既有类型错误；本次发现的新会话响应可选字段问题已修正。未对整个上游工程作类型清理。
- 浏览器截图及日志保留在 .local-build，仅用于开发验证，不进入 ZIP。

边界：覆盖了上述免登录核心流程，未遍历全部页面与团队功能。
此版本支持本机浏览器与原生置顶悬浮窗，不对局域网开放。
运行成品不需要 Node.js；从源码重新构建前端仍需要 Node.js。

## 悬浮窗更新验证

- Windows x64 原生小窗编译通过，无 Node.js 环境、带空格和中文的独立目录启动通过。
- 原生窗口验收通过：新增事项、勾选完成且保留原描述、51 项分页、搜索、独立浏览器会话、过期会话自动续期、置顶切换、收起展开和托盘恢复。
- 完整界面与悬浮窗使用独立刷新会话，避免互相消耗刷新令牌。
- 已查看窗口截图；输入提示、按钮、项目选择、分页及状态文字可见。
- 悬浮窗退出后，测试服务监听端口已释放。
- 验收日志：.local-build/floating-final-test.log；独立测试数据、截图与账号不进入发布包。
- 尚未在不同 DPI、多显示器及其他 Windows 机器上逐一验证；窗口恢复时会限制在可见工作区内。
## 2026-09-17 每日进展
- 浏览器成品验收通过：事项内保存今日进展和遗留问题、Ctrl+Enter 同日追加、刷新后两条记录均保留，HTML 特殊字符按文本显示。
- 悬浮窗验收通过：同一事项追加两条每日进展并由 API 读回；原事项描述仍保留。新增、完成、分页、搜索、会话续期回归通过。
- 新增 Vue 组件及关联评论组件 ESLint、新组件样式检查通过；前后端和悬浮窗构建通过。
- 日志：.local-build/progress-browser-test.log、.local-build/progress-native-test.log。

## 2026-09-17 已完成事项筛选
- 最终打包成品验证通过：隐藏已完成、开启后正确显示勾选状态、搜索已完成事项、恢复未完成、切换回隐藏、持久化筛选偏好。
- 原有新增、每日进展、保留描述、51 项分页、搜索、独立浏览器会话、续期、置顶、收起与恢复测试通过。
- 查看原生窗口截图，项目选择及“显示已完成”开关可见，无挤压遮挡。
- 日志：.local-build/completed-test.log。

## 2026-09-17 剪贴板贴图
- 网页成品 Playwright 验收通过：粘贴事件携带两张 PNG、出现双预览、保存进展、刷新后两张附件图片解码尺寸正确。日志 .local-build/paste-verified.log。
- 修复历史进展内相对附件地址未经过认证加载的问题，保存链接不绑定临时端口。
- 悬浮窗后端流程验证通过：两张 PNG 上传为独立附件，并写入每日进展。日志 .local-build/paste-native.log。原生剪贴板交互未做自动化键盘测试。
- ESLint、Stylelint、界面检查及完整构建通过。事项描述沿用已有编辑器的 Ctrl+V 图片上传能力。

## 2026-09-17 可配置自动保存
- 网页成品验收：配置 5 秒、自动新增、继续编辑更新同一条记录、无改动不重复请求、关闭后不自动提交、手动提交正常。
- 自动保存后刷新：恢复两张附件预览、原评论 ID 及保存状态，无重复提交；间隔设置保留。
- 事项描述独立成品测试：按配置自动写入，无变化时不重复请求。
- 悬浮窗回归验证通过，使用同一评论 ID 更新进展及图片，没有重复新增。原生定时器交互未单独做键盘自动化。
- 前后端及原生窗构建、ESLint、设置样式检查、界面机械检查通过。
- 日志：.local-build/autosave-browser.log、.local-build/autosave-description.log、.local-build/autosave-native.log。

## 2026-09-17 自定义数据目录
- 完整成品在程序目录外的中文带空格目录运行，悬浮窗核心回归及双图片保存通过，未在默认 data 下创建数据库。
- 改用相对路径指向同一数据目录，重启后原事项描述及两张附件仍可读取。
- 损坏 JSON 配置会启动失败，不会回退默认目录或生成新数据库。
- 配置脚本语法检查通过；配置只改变下次启动路径，不自动搬移数据。
- ZIP 不携带 tasktrace-settings.json 生效配置，保留升级时的目录选择。
- 日志：.local-build/data-directory-test.log、.local-build/data-restart.log、.local-build/data-invalid.log。

## 2026-09-17 子任务快捷入口
- 最终成品网页验收通过：创建子任务、继承项目、刷新保持关系、点击打开子任务独立进展；父事项完成状态不受影响。
- 原生回归通过：创建父子关系、独立完成子任务、原有图文进展与筛选等流程。
- 已查看子任务详情截图；ESLint、Stylelint、界面机械检查及完整构建通过。
- 日志：.local-build/subtasks-native.log、.local-build/subtasks-verified.log。

## 2026-09-17 完整模式已完成事项
- 修复首页固定 done=false 查询，提供未完成/全部/已完成选择并持久化。
- 成品浏览器测试通过：范围切换、已完成详情可打开、刷新后选择保留。
- 原有日期筛选 3 项单元测试、ESLint、Stylelint、构建通过。
- 因原悬浮窗正在运行，本轮生成到 dist/TaskTrace-release，再打包统一 ZIP；没有关闭用户程序。
- 日志：.local-build/web-completed-e2e.log、.local-build/web-completed-unit.log。

## 2026-09-17 项目默认展示模式
- 普通项目默认进入只读进展总览，父事项分区、多层子任务缩进，展示完成数量和最新进展。
- 全部/未完成/已完成筛选，按名称搜索分区，展开说明、附件图片和分页进展记录。
- 显式进入编辑模式，返回展示时重新读取项目；编辑视图之间切换保持编辑状态。
- 最终成品浏览器测试通过：分组、完成筛选、附件图片实际加载、模式切换和刷新。
- 分组单元测试 2 项通过；构建和样式检查通过，ESLint 无错误（只读 HTML 已使用 DOMPurify 清洗，保留 v-html 安全提示）。
- 日志：.local-build/browse-verified.log、.local-build/browse-unit.log、.local-build/browse-final-build.log。

## 2026-09-17 统一启动入口
- Start-TaskTrace.cmd 统一启动悬浮窗与完整浏览器界面，旧 Start-Floating.cmd 转发到统一入口。
- 完整界面使用独立浏览器登录会话，与悬浮窗共用服务和数据，不相互影响令牌刷新。
- 隔离成品自检通过：窗口功能、独立浏览器会话及退出清理；日志 .local-build/unified-launch-test.log。
- 构建通过并更新免安装 ZIP。

## 发布目录与首个 GitHub 版本
- 根目录 dist 已迁移为 Releases；Releases/TaskTrace-local 保留本地用户数据，ZIP 使用白名单生成。
- 首次发布为 v0.1.0-beta.1；源码与程序通过 SOURCE-COMMIT.txt 对应。
- 发布前 ESLint 无错误、Stylelint 通过；Go lint 因本机未安装 golangci-lint 无法运行，保留构建和成品验收结果。
- 上游 CI 的测试和发布依赖其专用基础设施，仅限上游仓库触发；本仓库首版手动构建发布。

## 2026-09-17 清理旧启动入口
- 删除已被统一入口替代的 Start-Floating.cmd，构建脚本不再复制或打包该文件。
- 清理本地旧 EXE 备份和过期打包暂存目录，保留用户数据与配置。
- 保留 Start-TaskTrace.cmd 和其依赖的 Launch-TaskTrace.ps1；配置入口继续保留。

## 2026-09-17 总表层级、完成筛选与关闭行为
- 项目明确作为总表，分别统计任务和子任务；每行标明任务/子任务层级。
- 列表编辑新增持久化的显示已完成任务开关，绕过上游默认视图的 done=false 固定筛选且不修改共享视图。
- 悬浮窗关闭按钮最小化至任务栏，只有托盘退出才停止服务。
- 成品浏览器验证：多个任务与子任务、完成任务和子任务显示/隐藏、刷新记忆、说明及图片展开通过。
- 原生自检验证关闭后最小化且 API 继续可用、恢复窗口与真正退出；12 项分组/过滤单元测试通过。
- 日志：.local-build/modes-e2e.log、.local-build/modes-native.log、.local-build/modes-unit.log。

## 2026-09-17 子任务进展四列表格
- 任务为分区标题，一个子任务一行；显示名称、描述、最新遗留事项和完整进展。
- 进展使用填写日期按新到旧排序，普通评论回退到创建日期，补录旧日期不会排到顶部。
- 图片继续通过只读富文本加载；任务自身的记录独立折叠，不混入子任务行数。
- 6 项解析/分组单元测试及成品浏览器验收通过，桌面与窄屏已查看。
- 日志：.local-build/table-unit.log、.local-build/table-e2e.log、.local-build/table-build.log。

## 2026-09-17 悬浮窗任务树
- 原生 TreeView 显示任务/多级子任务，支持展开、收起和键盘方向键操作。
- 展开状态保存在本机数据目录，刷新与重启保留；搜索临时展开并保留父任务上下文。
- 按 50 个顶层任务组分页，父子任务不跨页；勾选完成保持各任务独立。
- 原生自检通过：多级缩进、展开状态文件恢复、搜索层级、已完成父任务保留未完成子任务及原有进展、完成、分页、窗口行为。
- 已检查真实层级截图；日志 .local-build/tree-native-verified.log。

## 2026-09-17 完整界面分级概览
- 根任务分区与多级子任务均支持独立展开、收起，浏览器保存收起状态。
- 搜索保留并展开祖先链，清空后恢复；完成筛选保留匹配子任务的父级上下文。
- 9 项单元测试通过；成品浏览器验收通过，包括真实多级缩进、持久化、搜索、完成筛选、原有图片/进展与编辑切换；已查看桌面与窄屏截图。
- ESLint 无错误（19 项已有警告），Stylelint 通过；全库类型检查仍有已有错误，本次修改文件无类型错误。
- 日志：.local-build/overview-tree-test.log、.local-build/overview-tree-e2e-final.log、.local-build/overview-tree-build.log。

## 2026-09-17 跨电脑错误诊断
- 悬浮窗提供完整可复制异常、内层异常、Windows 错误码和本地诊断日志；浏览器打开失败单独标明阶段并弹出详情。
- 启动脚本提供可滚动、可复制错误详情、阶段和日志路径，数据目录不可写时回退到程序/临时目录。
- 注入 Windows 1155 错误验证完整诊断和会话 URL 脱敏；原生完整自检通过。
- 无效配置模拟启动失败，验证退出码 1 和包含异常栈的日志；PowerShell 语法与成品构建通过。
- 日志：.local-build/startup-native.log、.local-build/startup-error-verified.log、.local-build/startup-build.log。
- 此版本修复错误不可见问题；用户另一台 Win11 的实际启动错误原因需要该机详情确认。

## 2026-09-17 悬浮窗启动兼容性
- 复现 Start-Process 将 Win32Exception 包装为 InvalidOperationException 丢失 NativeErrorCode；相同无效程序使用 Process.Start 保留原生错误 216。
- 悬浮窗改为 ProcessStartInfo 显式直接启动，关闭 ShellExecute、设置工作目录；路径尾部分隔符使用反斜杠加点避免引号转义问题。
- 日志保留 ErrorRecord、错误 ID、PowerShell 版本、系统位数及原始错误链；为常见 Windows 错误提供对应说明。
- 中文空格目录完整原生自检通过。实际启动脚本注入无效悬浮窗程序，确认启动阶段、原生错误码及失败后后台退出。
- 日志：.local-build/launch-probe/result.log、.local-build/launch-compat-native.log、.local-build/launch-invalid-binary.log。
- 已确认并修复启动方式与诊断缺陷；仍不能在本机证明另一台 Win11 的特定环境问题完全消失。

## 2026-09-17 小羊驼启动图标
- 复用 frontend/public/favicon.ico，嵌入 TaskTrace.exe 和 TaskTrace-floating.exe；悬浮窗及托盘读取嵌入图标。
- 新增轻量原生启动入口，直接调用统一 PowerShell 启动脚本，保留原 CMD 入口。
- 通过 TaskTrace.exe --self-test 运行完整原生验收通过；从两个成品 EXE 提取图标并确认均为小羊驼。
- 发布白名单增加 TaskTrace.exe，仍不打包任何运行数据或本机配置。

## 2026-09-17 EXE 无终端启动
- TaskTrace.exe 作为启动入口，删除旧 Start-TaskTrace.cmd，构建时清理旧入口。
- 工作区初始化和后台服务均使用 ProcessStartInfo，UseShellExecute=false、CreateNoWindow=true；标准输出及错误异步写日志，退出后回收流。
- 新 EXE 入口完整原生自检通过；50ms 采样未检测到新 PowerShell/conhost/server 可见主窗口，后台日志保留。
- 日志：.local-build/no-console-verified.log、.local-build/no-console-build.log。

## 2026-09-17 子任务名称编辑
- 完整界面子任务列表支持改名、取消、空名称校验与失败保留输入；使用 API v2 JSON Patch 只更新 title。
- 悬浮窗子任务窗口支持选中后改名、回车保存与失败保留输入。
- 原生自检验证改名持久化及关系保留；成品浏览器验收通过空名称、取消、保存、刷新后名称和上级关系。
- ESLint 0 错误（19 项已有警告）；日志 .local-build/rename-e2e.log、.local-build/rename-lint.log。

## 2026-09-17 子任务遗留事项汇总
- 任务自身进展增加全部后代的最新遗留事项汇总，保留自身内容、来源名称和已完成标记；使用完整分组，不受展开/筛选影响。
- 延迟读取、六请求并发限制、全评论分页、读取失败提示并支持重试；富文本继续通过只读组件处理。
- 成品浏览器验收通过：自身/子任务/下级/已完成来源、收起与筛选、最新空记录移除旧事项，以及父任务评论不被改写。已查看桌面和窄屏截图。
- ESLint 无错误（19 项已有警告），Stylelint 通过；日志 .local-build/summary-e2e.log、.local-build/summary-build.log。
