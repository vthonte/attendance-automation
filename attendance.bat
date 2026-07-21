@echo off
setlocal

set "BASE=%~dp0"

cd /d "%BASE%"
set "ATTENDANCE_BASE_DIR=%BASE%"
set "ATTENDANCE_DATA_DIR=%BASE%data"
node "%BASE%src\attendance.js"
