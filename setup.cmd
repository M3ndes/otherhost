@echo off
setlocal

set "state_dir=%LOCALAPPDATA%\devbox-bridge"
if not exist "%state_dir%" mkdir "%state_dir%" >nul 2>&1
>"%state_dir%\setup.status" echo launching

wscript.exe //B //NoLogo "%~dp0scripts\launch-windows-setup.vbs" "%~dp0setup.ps1" %*
if errorlevel 1 (
  >"%state_dir%\setup.status" echo failed
  echo Failed to hand setup to the Windows elevation broker.
  exit /b 1
)

echo Devbox Bridge setup was handed to the Windows elevation broker.
echo Status: %state_dir%\setup.status
echo Log:    %state_dir%\setup.log
exit /b 0
