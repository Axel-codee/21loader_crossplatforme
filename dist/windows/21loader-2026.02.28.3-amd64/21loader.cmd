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

pushd "%APP_DIR%" || (
  echo Unable to enter app directory: "%APP_DIR%"
  exit /b 1
)

start "" /B "%APP_DIR%\21loader-server.exe" --host "%LOADER21_HOST%" --port "%LOADER21_PORT%" >> "%LOG_FILE%" 2>&1
timeout /t 1 /nobreak >nul
start "" "http://%LOADER21_HOST%:%LOADER21_PORT%"

popd
endlocal
