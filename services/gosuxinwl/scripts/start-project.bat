@echo off
chcp 65001 >nul
setlocal
title 考勤管理系统 - 一键启动
set "PROJECT_DIR=E:\PUTY_AI\kaoqinstyem\gosuxinwl"
set "PROJECT_URL=https://kf.szputy.com:9880/suxinweb/"

echo 正在启动考勤管理系统，请稍候...
echo.
if not exist "%PROJECT_DIR%\scripts\start-local.ps1" goto missing
cd /d "%PROJECT_DIR%"
if errorlevel 1 goto failed
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%PROJECT_DIR%\scripts\start-local.ps1"
if errorlevel 1 goto failed

echo.
echo 启动成功。重复运行不会重启已在运行的服务。
echo 访问地址：%PROJECT_URL%
if /i "%~1"=="--check" exit /b 0
start "" "%PROJECT_URL%"
echo.
echo 按任意键关闭本窗口，服务会继续在后台运行。
pause >nul
exit /b 0

:missing
echo 未找到项目启动脚本，请检查目录：%PROJECT_DIR%
goto error_exit

:failed
echo.
echo 启动失败，请查看上方错误信息。
echo 宿主日志：%PROJECT_DIR%\runtime\core-running-stdout.log
echo 错误日志：%PROJECT_DIR%\runtime\core-running-stderr.log
echo HTTPS 日志：%PROJECT_DIR%\runtime\https-stderr.log

:error_exit
if /i "%~1"=="--check" exit /b 1
echo.
echo 按任意键关闭本窗口。
pause >nul
exit /b 1
