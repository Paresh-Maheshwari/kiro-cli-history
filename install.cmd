@echo off
rem Install kiro-cli-history on Windows (double-click or run from cmd).
rem Uses install.ps1 next to this file if present, otherwise the latest from GitHub.
setlocal
set "PS1=%~dp0install.ps1"
if exist "%PS1%" (
    powershell -NoProfile -ExecutionPolicy Bypass -File "%PS1%"
) else (
    powershell -NoProfile -ExecutionPolicy Bypass -Command "irm https://raw.githubusercontent.com/Paresh-Maheshwari/kiro-cli-history/main/install.ps1 | iex"
)
set "RC=%ERRORLEVEL%"
if /i not "%~1"=="/nopause" pause
exit /b %RC%
