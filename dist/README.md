# 从源码生成 TaskTrace 免安装程序

双击 `Install-TaskTrace.cmd`，先选择安装目录。路径输入框支持直接粘贴完整路径，也可以点击“浏览”选择目录；尚不存在的目录会在安装时自动创建。确认路径后，安装器首先检查该目录中的 TaskTrace 是否仍在运行；正在运行时会立即提示退出，并且不会开始下载、编译或覆盖文件。如果检测到目录中已有但未运行的 TaskTrace，工具会显示现有版本并要求确认，明确提示只覆盖程序并保留配置和数据。工具随后检查全部构建依赖；只有检查通过后才开始生成并安装程序。成功、缺少依赖或构建失败都会弹出结果窗口。

运行检查兼容 32 位安装进程检测 64 位程序，也会识别目录联接、短路径等指向同一安装目录的路径，并在构建前检查目标程序文件是否被占用或无法写入。无法读取某个 TaskTrace 进程路径时会提前说明，不再忽略该进程继续构建；已确认运行在另一目录中的程序不会阻止本次安装。构建后、覆盖文件前仍会再次检查，防止生成期间重新启动了目标程序。

默认安装目录为：

```text
dist\TaskTrace-local\TaskTrace.exe
```

如果选择的目录中已有 TaskTrace，安装器只替换程序文件，绝不会修改 `tasktrace-settings.json`、`data`、`teamData`、`.cache`、`backups` 以及其他用户文件。构建过程在系统临时目录完成，不会把构建电脑上的测试数据带入目标目录。

也可以从命令行直接指定安装目录：

```powershell
dist\Install-TaskTrace.cmd -InstallDirectory "D:\Apps\TaskTrace"
```

如果要手工复制到另一台电脑，安装器仍会生成安全复制包：

```text
dist\TaskTrace-program-files.zip
```

这个压缩包只包含程序文件，明确排除 `data`、`teamData`、`.cache`、`backups` 和 `tasktrace-settings.json`。退出目标电脑上正在运行的 TaskTrace 后，将压缩包内容解压并覆盖到原程序目录；原数据库、团队协作仓库、草稿缓存、备份和本机数据目录设置都会保留。不要把已经运行过的 `dist\TaskTrace-local` 整个目录复制到另一台电脑，因为其中可能带有构建电脑生成的数据库和本机配置。

如果旧数据仍在其他目录，启动 TaskTrace 后进入“设置 → 数据保护与恢复”。程序可以自动检测旧版目录，也可以按用户填写的完整路径检测；确认导入后会复制到新的纯数据目录、校验数据库并备份配置，不会覆盖当前数据。

## 构建依赖

- Windows 10/11 x64、Windows PowerShell 5.1 或 PowerShell 7
- Node.js 24 或更新版本
- pnpm 11.26.0 或更新版本
- Go 1.27.0 或更新版本
- Windows x64 GCC，推荐 MSYS2 UCRT64 GCC
- GNU Binutils `strip`（通常随 GCC 提供）
- .NET Framework 4.8 C# 编译器
- 本机未缓存项目依赖时，可访问 npm 和 Go 依赖源；已经缓存完整时不会重复下载

源码可以来自 Git 克隆或 GitHub 的 Source code ZIP；生成过程不依赖 `.git` 信息，也不要求安装 Git。普通 install 构建会先读取 GitHub Release API，受限时自动改读公开 Release 订阅源，并生成形如 `v0.1.0-beta.14.source.20260923153000` 的源码版号，因此检查更新时源码版高于构建时已有的 Release；将来发布更高版本后仍会正常提示。GitHub 暂时不可访问时使用源码内置的 Release 基准。安装器每次启动都会重新读取当前用户和系统保存的 PATH，再从 PATH、`where`、Windows 常见安装目录、Node.js/Go 注册表、WinGet、Volta、Scoop、NVM、PNPM_HOME 和 Corepack 查找已安装工具，并在日志中列出实际使用的程序路径。即使双击安装器的资源管理器仍保留安装工具之前的旧环境，也能识别新安装的 Node.js、pnpm 和 Go。

安装器会针对 npm 注册表和 Go 模块代理读取 Windows 当前系统代理，并把实际采用的代理或直接连接状态显示在窗口及 `install.log` 中。依赖下载信息实时输出：

- 开始下载时显示依赖名称和序号。
- 完成时显示大小、该依赖的下载速度、累计下载量和平均速度。
- 已缓存的依赖不会重复下载；发生瞬时网络中断时，Go 模块会自动重试三次。
- 完整的前端锁定依赖和 Go 模块列表写入 `dist\install-dependencies.txt`。

只检查环境而不构建：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File dist/Install-TaskTrace-Engine.ps1 -CheckOnly
```

如果失败，窗口会列出缺少或版本不合格的依赖以及处理方法，完整记录保存在 `dist/install.log`。

这些依赖只用于从源码生成程序。生成完成后，运行 `TaskTrace-local\TaskTrace.exe` 不依赖 Node.js、pnpm、Go、GCC 或开发环境。

`TaskTrace-local` 中运行产生的数据库、图片、团队数据和个人设置只保存在本机，不会被 Git 跟踪。
