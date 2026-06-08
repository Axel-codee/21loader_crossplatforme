@echo off
setlocal

set "SCRIPT_DIR=%~dp0"
set "APP_DIR=%SCRIPT_DIR%app"

if not defined LOADER21_HOST set "LOADER21_HOST=127.0.0.1"
if not defined LOADER21_PORT set "LOADER21_PORT=8080"

set "LOG_DIR=%APPDATA%\21loader\Logs\21loader"
if not exist "%LOG_DIR%" mkdir "%LOG_DIR%" >nul 2>&1
set "LOG_FILE=%LOG_DIR%\server.log"

if not exist "%APP_DIR%\21loader-server.exe" (
  echo Executable not found: "%APP_DIR%\21loader-server.exe"
  exit /b 1
)

if /I "%~1"=="update" goto run_foreground
if /I "%~1"=="version" goto run_foreground
if /I "%~1"=="--version" goto run_foreground

pushd "%APP_DIR%" || (
  echo Unable to enter app directory: "%APP_DIR%"
  exit /b 1
)

start "" /B "%APP_DIR%\21loader-server.exe" --open --host "%LOADER21_HOST%" --port "%LOADER21_PORT%" %* >> "%LOG_FILE%" 2>&1

popd
endlocal
exit /b 0

:run_foreground
pushd "%APP_DIR%" || (
  echo Unable to enter app directory: "%APP_DIR%"
  exit /b 1
)
"%APP_DIR%\21loader-server.exe" %*
set "EXIT_CODE=%ERRORLEVEL%"
popd
endlocal & exit /b %EXIT_CODE%
