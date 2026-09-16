@echo off
cd /d "%~dp0"
start "" powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File "%~dp0Launch-TaskTrace.ps1" -Floating -OpenBrowser
