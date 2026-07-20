@echo off
setlocal

set "BASE=%~dp0"

cd /d "%BASE%"
node "%BASE%attendance.js"
