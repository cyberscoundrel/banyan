@echo off
REM Banyan Node Debug Startup Script (Windows Batch)
REM This script launches the banyan node with a debug/test configuration
REM for local development and testing.

setlocal enabledelayedexpansion

REM Get the directory where this script is located
set SCRIPT_DIR=%~dp0

REM Path to the banyan executable
set BANYAN_EXEC=%SCRIPT_DIR%banyan.exe

REM Debug directories (relative to script location)
set DEBUG_DIR=%SCRIPT_DIR%..\debug\win
set DEFAULT_SERVICES_DIR=%DEBUG_DIR%\services
set DEFAULT_ADDONS_DIR=%DEBUG_DIR%\addons
set DEFAULT_FIGS_DIR=%DEBUG_DIR%\figs

REM Check if banyan executable exists
if not exist "%BANYAN_EXEC%" (
    echo Error: banyan.exe not found at %BANYAN_EXEC%
    echo Make sure you're running this script from the build output directory.
    exit /b 1
)

REM Default configuration
set LISTEN=/ip4/127.0.0.1/tcp/0
set SERVICES_DIR=%DEFAULT_SERVICES_DIR%
set ADDONS_DIR=%DEFAULT_ADDONS_DIR%
set FIGS_DIR=%DEFAULT_FIGS_DIR%
set EXTRA_ARGS=
set BARE_MODE=0

REM Parse arguments
:parse_args
if "%~1"=="" goto done_parse
if /i "%~1"=="--help" goto show_help
if /i "%~1"=="-h" goto show_help
if /i "%~1"=="--bare" (
    set BARE_MODE=1
    shift
    goto parse_args
)
if /i "%~1"=="--listen" (
    set LISTEN=%~2
    shift
    shift
    goto parse_args
)
if /i "%~1"=="--config" (
    set EXTRA_ARGS=!EXTRA_ARGS! -config="%~2"
    shift
    shift
    goto parse_args
)
if /i "%~1"=="--privkey" (
    set EXTRA_ARGS=!EXTRA_ARGS! -privkey="%~2"
    shift
    shift
    goto parse_args
)
if /i "%~1"=="--services-dir" (
    set SERVICES_DIR=%~2
    shift
    shift
    goto parse_args
)
if /i "%~1"=="--addons-dir" (
    set ADDONS_DIR=%~2
    shift
    shift
    goto parse_args
)
if /i "%~1"=="--figs-dir" (
    set FIGS_DIR=%~2
    shift
    shift
    goto parse_args
)
if /i "%~1"=="--no-crypto" (
    set EXTRA_ARGS=!EXTRA_ARGS! -no-crypto
    shift
    goto parse_args
)
if /i "%~1"=="--disable-dht" (
    set EXTRA_ARGS=!EXTRA_ARGS! -disable-dht
    shift
    goto parse_args
)
if /i "%~1"=="--allow-expired" (
    set EXTRA_ARGS=!EXTRA_ARGS! -allow-expired-figs
    shift
    goto parse_args
)
if /i "%~1"=="--allow-insecure" (
    set EXTRA_ARGS=!EXTRA_ARGS! -allow-insecure-figs
    shift
    goto parse_args
)
shift
goto parse_args

:show_help
echo Banyan Debug Startup Script
echo.
echo Usage: start-debug.bat [options]
echo.
echo Options:
echo   --help, -h          Show this help message
echo   --bare              Run with no arguments (use banyan defaults)
echo   --listen ADDR       Listen address (default: /ip4/127.0.0.1/tcp/0)
echo   --config PATH       Config file to use
echo   --privkey PATH      Private key file
echo   --services-dir PATH Services directory (default: ../debug/win/services)
echo   --addons-dir PATH   Addons directory (default: ../debug/win/addons)
echo   --figs-dir PATH     Figs directory (default: ../debug/win/figs)
echo   --no-crypto         Disable encryption for testing
echo   --disable-dht       Disable DHT discovery
echo   --allow-expired     Allow expired figs for testing
echo   --allow-insecure    Allow insecure figs for testing
echo.
echo Examples:
echo   start-debug.bat
echo   start-debug.bat --no-crypto --disable-dht
echo   start-debug.bat --config my-config.json
echo   start-debug.bat --bare
exit /b 0

:done_parse

REM Handle bare mode
if %BARE_MODE%==1 (
    echo === Banyan Node ^(Bare Mode^) ===
    echo Executable: %BANYAN_EXEC%
    echo ================================
    echo.
    echo Starting Banyan node...
    "%BANYAN_EXEC%"
    exit /b %ERRORLEVEL%
)

REM Display startup info
echo === Banyan Node Debug Mode ===
echo Executable:   %BANYAN_EXEC%
echo Services Dir: %SERVICES_DIR%
echo Addons Dir:   %ADDONS_DIR%
echo Figs Dir:     %FIGS_DIR%
echo Listen:       %LISTEN%
echo Extra args:   %EXTRA_ARGS%
echo ==============================
echo.

REM Start the node
echo Starting Banyan node...
"%BANYAN_EXEC%" -listen="%LISTEN%" -services-dir="%SERVICES_DIR%" -addons-dir="%ADDONS_DIR%" -figs-dir="%FIGS_DIR%" %EXTRA_ARGS%

