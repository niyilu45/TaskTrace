@echo off
setlocal
chcp 65001 >nul
title TaskTrace Portable Builder
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0Install-TaskTrace-Engine.ps1" -Interactive %*
set "TASKTRACE_INSTALL_EXIT=%ERRORLEVEL%"
if "%TASKTRACE_INSTALL_EXIT%"=="1223" exit /b %TASKTRACE_INSTALL_EXIT%
echo.
if not "%TASKTRACE_INSTALL_EXIT%"=="0" goto install_failed
echo 安装完成，可以关闭本窗口。
goto wait_to_close
:install_failed
echo Build failed. Review the result dialog or install.log for details.
:wait_to_close
echo.
echo 按任意键关闭本窗口...
pause >nul
exit /b %TASKTRACE_INSTALL_EXIT%
