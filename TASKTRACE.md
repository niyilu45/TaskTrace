# TaskTrace

基于 Vikunja 二次开发的团队事项跟踪软件。

## 已确认需求

- 支持大量事项的列表查看与管理。
- 持续记录事项进展，保留历史记录。
- 独立跟踪遗留问题及其解决状态。
- 支持在进展记录中粘贴、上传和浏览多张图片。
- 支持团队协作、任务分工与项目权限。
- 支持 Windows 桌面常驻置顶小窗，切换其他软件仍可见。
- 使用免费开源代码作为开发基础，遵守各部分许可证。

## 工程状态

- 工程目录：D:\program\AI\TaskTrace
- 开发分支：codex/tasktrace
- 官方上游：https://github.com/go-vikunja/vikunja.git
- 2026-09-16：已导入 upstream/main 源码；发布仓库使用 TaskTrace 独立提交历史，不上传上游 Git 历史。
- 上游基准提交：5f3504827990df58bef84b3a5d8c6ab398534c0b。
- 已阅读上游 AGENTS.md；保留原始代码和许可证。
- 已完成本机免安装浏览器体验包；运行不依赖 Node.js，支持本地 SQLite 与图片持久化。
- 本机体验已支持免注册、免输入账号密码，启动即进入本地工作区，自动续期并保留旧单账号数据。
- 源码安装入口：dist/Install-TaskTrace.cmd；本机生成目录：dist/TaskTrace-local；正式发布包：Releases/TaskTrace-local-windows-x64.zip；测试记录：portable/VERIFICATION.md。
- 已加入原生 Windows 置顶悬浮窗：免登录、快速新增、完成、项目选择、搜索分页、收起和托盘恢复。
- 已加入基于 Windows `teamData` 共享文件夹的局域网团队协作：子树任务链接导入、按用户名多设备合并、评论/进展及图片附件自动合并、冲突集中处理、成员通知和个人优先级隔离。
- 已支持网页及悬浮窗快捷记录每日进展，按日期追加历史，可填写遗留问题 / 下一步；保存于现有事项评论，支持后续编辑和加图。

## 下一步

在已验证的本机体验基础上，继续完善图文进展日志和遗留问题跟踪。



## 本地测试约定
- 用户通过 `dist/Install-TaskTrace.cmd` 检查依赖并生成 `dist/TaskTrace-local`，再直接测试其中的程序；生成的二进制文件不提交。
- 更新时保留已有 `data` 和 `tasktrace-settings.json`，不停止用户正在运行的程序。
- 自动验收使用 `.local-build` 下的独立数据，禁止将临时数据、登录会话、运行配置、截图及构建产物提交到 Git。
- `dist/` 只跟踪安装脚本、说明和输出目录忽略规则；`dist/TaskTrace-local`、`/.local-build/` 和前端测试产物均不提交。提交前确认没有跟踪二进制文件或运行数据。
