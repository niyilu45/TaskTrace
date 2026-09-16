@echo off
cd /d "%~dp0"
start "" powershell.exe -NoLogo -NoProfile -STA -ExecutionPolicy Bypass -WindowStyle Hidden -File "%~dp0Configure-TaskTrace.ps1"
