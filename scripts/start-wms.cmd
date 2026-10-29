@echo off
rem Starts WittyMediaServer in this window (portable mode: data next to the exe).
cd /d "%~dp0"
set WMS_DATA_DIR=%~dp0data
wms.exe serve
pause
