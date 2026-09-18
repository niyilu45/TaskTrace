@echo off
chcp 65001 >nul
title TaskTrace 免安装程序生成工具
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0Install-TaskTrace.ps1"
set "TASKTRACE_INSTALL_EXIT=%ERRORLEVEL%"
echo.
if not "%TASKTRACE_INSTALL_EXIT%"=="0" echo 生成失败，请按照上方提示补齐依赖后重试。
pause
exit /b %TASKTRACE_INSTALL_EXIT%
