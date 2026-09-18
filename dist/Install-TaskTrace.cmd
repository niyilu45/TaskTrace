@echo off
setlocal
chcp 65001 >nul
title TaskTrace Portable Builder
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0Install-TaskTrace-Engine.ps1" -Interactive
set "TASKTRACE_INSTALL_EXIT=%ERRORLEVEL%"
echo.
if not "%TASKTRACE_INSTALL_EXIT%"=="0" goto install_failed
echo Build completed. Review the result dialog before closing this window.
goto wait_to_close
:install_failed
echo Build failed. Review the result dialog or install.log for details.
:wait_to_close
echo.
echo Press any key to close this window...
pause >nul
exit /b %TASKTRACE_INSTALL_EXIT%
