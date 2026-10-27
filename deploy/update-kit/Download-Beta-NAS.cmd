@echo off
setlocal
chcp 65001 >nul
pushd "%~dp0" || exit /b 1
"%~dp0bin\layerhub-update-windows-amd64.exe" download --channel beta --platform linux --output "%~dp0downloads" %*
set "KIT_EXIT=%ERRORLEVEL%"
echo.
pause
popd
exit /b %KIT_EXIT%
