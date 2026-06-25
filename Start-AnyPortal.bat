@echo off
chcp 65001 >nul 2>&1
setlocal EnableDelayedExpansion
title AnyPortal Launcher v2.0
cls

echo.
echo  ╔══════════════════════════════════════════════════════════════╗
echo  ║          🚀  AnyPortal Automated Launcher  🚀              ║
echo  ║          Zero-Config Game Streaming Portal                  ║
echo  ╚══════════════════════════════════════════════════════════════╝
echo.

REM ═══════════════════════════════════════════════════════════════
REM  Step 1: Kill residual processes
REM ═══════════════════════════════════════════════════════════════
echo  🧹 [Step 1/5] Cleaning up residual processes...
taskkill /F /IM AnyPortal-CLI.exe >nul 2>&1
taskkill /F /IM AnyPortal-Tray.exe >nul 2>&1
taskkill /F /IM sunshine.exe >nul 2>&1
echo  ✅  Residual processes terminated.
echo.

REM ═══════════════════════════════════════════════════════════════
REM  Step 2: Nuke stale tsnet cache
REM ═══════════════════════════════════════════════════════════════
echo  🗑️  [Step 2/5] Removing stale tsnet cache directory...
if exist ".tsnet-anyportal-host" (
    rmdir /S /Q ".tsnet-anyportal-host" >nul 2>&1
    echo  ✅  Cache folder ".tsnet-anyportal-host" deleted.
) else (
    echo  ℹ️  No stale cache found. Skipping.
)
echo.

REM ═══════════════════════════════════════════════════════════════
REM  Step 3: Prompt for new Ephemeral Auth Key
REM ═══════════════════════════════════════════════════════════════
echo  🔑 [Step 3/5] Tailscale Ephemeral Auth Key Required
echo  ───────────────────────────────────────────────────
echo  Generate a new key at:
echo  https://login.tailscale.com/admin/settings/keys
echo  (Select "Auth Key" and check "Ephemeral")
echo.
set /p "AUTH_KEY=  👉 Paste your new Auth Key here: "

if "!AUTH_KEY!"=="" (
    echo.
    echo  ❌ ERROR: Auth key cannot be empty. Aborting.
    pause
    exit /b 1
)

REM Basic prefix validation
echo !AUTH_KEY! | findstr /B "tskey-auth-" >nul 2>&1
if errorlevel 1 (
    echo.
    echo  ⚠️  WARNING: Key does not start with "tskey-auth-".
    echo     This may not be a valid Tailscale auth key.
    echo.
    set /p "CONTINUE=  ❓ Continue anyway? (Y/N): "
    if /I not "!CONTINUE!"=="Y" (
        echo  🛑 Aborted by user.
        pause
        exit /b 1
    )
)
echo.

REM ═══════════════════════════════════════════════════════════════
REM  Step 4: Patch config.json via PowerShell
REM ═══════════════════════════════════════════════════════════════
echo  📝 [Step 4/5] Updating config.json with new auth key...

powershell -NoProfile -ExecutionPolicy Bypass -Command ^
    "$cfg = Get-Content -Raw -Path '.\config.json' | ConvertFrom-Json; ^
     $cfg.auth_key = '%AUTH_KEY%'; ^
     $cfg | ConvertTo-Json -Depth 10 | Set-Content -Path '.\config.json' -Encoding UTF8; ^
     Write-Host '  ✅  config.json updated successfully.'"

if errorlevel 1 (
    echo  ❌ ERROR: Failed to update config.json. Is the file valid JSON?
    pause
    exit /b 1
)
echo.

REM ═══════════════════════════════════════════════════════════════
REM  Step 5: Launch AnyPortal
REM ═══════════════════════════════════════════════════════════════
echo  🚀 [Step 5/5] Launching AnyPortal-CLI.exe ...
echo  ═══════════════════════════════════════════════════════════════
echo.

.\AnyPortal-CLI.exe

echo.
echo  🏁 AnyPortal has exited.
pause
