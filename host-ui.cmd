@echo off
setlocal
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0host-ui.ps1" %*
exit /b %ERRORLEVEL%
