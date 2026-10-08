# 从源码生成 TaskTrace 免安装程序

双击 `Install-TaskTrace.cmd`。工具会先检查全部构建依赖；只有检查通过后才开始生成程序。成功、缺少依赖或构建失败都会弹出结果窗口。

生成结果位于：

```text
dist\TaskTrace-local\TaskTrace.exe
```

如果要复制到另一台电脑，或覆盖另一套已经在使用的 TaskTrace，请使用安装器同时生成的安全复制包：

```text
dist\TaskTrace-program-files.zip
```

这个压缩包只包含程序文件，明确排除 `data`、`teamData`、`.cache` 和 `tasktrace-settings.json`。退出目标电脑上正在运行的 TaskTrace 后，将压缩包内容解压并覆盖到原程序目录；原数据库、团队协作仓库、草稿缓存和本机数据目录设置都会保留。不要把已经运行过的 `dist\TaskTrace-local` 整个目录复制到另一台电脑，因为其中可能带有构建电脑生成的空数据库和本机配置。

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
