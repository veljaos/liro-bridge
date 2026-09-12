@echo off
REM One command, no arguments. Run it WHILE the window is still open,
REM before closing anything, when a document dragged onto Liro Bridge
REM does nothing. It changes nothing and writes a report to the Desktop.
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0dragreport.ps1"
pause
