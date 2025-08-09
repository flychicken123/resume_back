@echo off
echo Installing backend dependencies...

REM Check if Go is installed
go version >nul 2>&1
if errorlevel 1 (
    echo Error: Go is not installed. Please install Go first.
    pause
    exit /b 1
)

REM Install Go dependencies
echo Installing Go dependencies...
go mod tidy

echo Backend installation complete!
echo To run the backend: go run main.go
pause




