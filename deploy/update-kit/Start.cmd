@echo off
setlocal
chcp 65001 >nul
pushd "%~dp0" || exit /b 1
"%~dp0bin\layerhub-update-windows-amd64.exe" menu
set "KIT_EXIT=%ERRORLEVEL%"
pause
popd
exit /b %KIT_EXIT%
