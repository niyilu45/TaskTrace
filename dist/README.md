# 从源码生成 TaskTrace 免安装程序

双击 `Install-TaskTrace.cmd`。工具会先检查全部构建依赖；只有检查通过后才开始生成程序。成功、缺少依赖或构建失败都会弹出结果窗口。

生成结果位于：

```text
dist\TaskTrace-local\TaskTrace.exe
```

## 构建依赖

- Windows 10/11 x64、Windows PowerShell 5.1 或 PowerShell 7
- Git for Windows，并通过 `git clone` 获取本仓库
- Node.js 24 或更新版本
- pnpm 11.26.0 或更新版本
- Go 1.27.0 或更新版本
- Windows x64 GCC，推荐 MSYS2 UCRT64 GCC
- .NET Framework 4.8 C# 编译器
- 首次构建时可访问 npm 和 Go 依赖源

只检查环境而不构建：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File dist/Install-TaskTrace-Engine.ps1 -CheckOnly
```

如果失败，窗口会列出缺少或版本不合格的依赖以及处理方法，完整记录保存在 `dist/install.log`。

这些依赖只用于从源码生成程序。生成完成后，运行 `TaskTrace-local\TaskTrace.exe` 不依赖 Node.js、pnpm、Go、GCC 或开发环境。

`TaskTrace-local` 中运行产生的数据库、图片、团队数据和个人设置只保存在本机，不会被 Git 跟踪。
