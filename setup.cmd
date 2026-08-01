@echo off
setlocal

if /I "%DEVBOX_SETUP_ATTACHED%"=="1" goto attached

set "state_dir=%LOCALAPPDATA%\devbox-bridge"
if not exist "%state_dir%" mkdir "%state_dir%" >nul 2>&1
>"%state_dir%\setup.status" echo launching

set "DEVBOX_SETUP_ATTACHED=1"
start "Devbox Bridge Setup" /D "%~dp0" "%ComSpec%" /d /c call "%~f0" %*
if errorlevel 1 (
  echo Failed to launch the Devbox Bridge setup window.
  exit /b 1
)

echo Devbox Bridge setup started in its own Windows window.
echo Status: %state_dir%\setup.status
echo Log:    %state_dir%\setup.log
exit /b 0

:attached
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0setup.ps1" %*
exit /b %errorlevel%
