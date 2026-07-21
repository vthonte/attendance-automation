import fs from "fs";
import path from "path";
import { exec, spawn } from "child_process";

export function getChromePath() {
  if (process.platform === "win32") {
    return path.join(
      process.env.LOCALAPPDATA,
      "Google",
      "Chrome",
      "Application",
      "chrome.exe",
    );
  }
  if (process.platform === "darwin") return "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
  return "/usr/bin/google-chrome";
}

export function getChromeDebugProfile(base) {
  if (process.platform === "win32") return path.join(process.env.LOCALAPPDATA, "ChromeDebug");
  return path.join(process.env.HOME || base, ".attendance", "ChromeDebug");
}

export function getToastCommand(base, statusFile) {
  if (process.env.ATTENDANCE_PACKAGED === "1") {
    return { command: process.execPath, args: [statusFile] };
  }

  const electron = process.platform === "win32"
    ? path.join(base, "node_modules", "electron", "dist", "electron.exe")
    : path.join(base, "node_modules", ".bin", "electron");
  return {
    command: electron,
    args: [
      path.join(base, "src", "notifyToast.js"),
      statusFile,
    ],
  };
}

export function runPlatformCommand(command) {
  return new Promise((resolve) => exec(command, () => resolve()));
}

export async function stopChromeForAttendance(profile) {
  if (process.platform === "win32") {
    const escaped = profile.replace(/'/g, "''");
    await runPlatformCommand(`powershell -NoProfile -WindowStyle Hidden -Command "$profile='${escaped}'; $owners=Get-NetTCPConnection -LocalPort 9222 -State Listen -ErrorAction SilentlyContinue | Select-Object -ExpandProperty OwningProcess -Unique; foreach($ownerPid in $owners){ Stop-Process -Id $ownerPid -Force -ErrorAction SilentlyContinue }; Get-CimInstance Win32_Process -Filter \"Name = 'chrome.exe'\" -ErrorAction SilentlyContinue | Where-Object { $_.CommandLine -like ('*' + $profile + '*') } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }"`);
  }
}

export async function focusChrome() {
  if (process.platform !== "win32") return;
  await runPlatformCommand("powershell -NoProfile -WindowStyle Hidden -Command \"$ws=New-Object -ComObject WScript.Shell; if (-not $ws.AppActivate('Keka')) { $null=$ws.AppActivate('Google Chrome') }\"");
}

export function spawnToast(base, statusFile) {
  const { command, args } = getToastCommand(base, statusFile);
  if (process.env.ATTENDANCE_PACKAGED === "1") args.unshift("--toast");
  const { ELECTRON_RUN_AS_NODE, ...toastEnvironment } = process.env;
  const toast = spawn(command, args, {
    cwd: base,
    detached: true,
    stdio: "ignore",
    windowsHide: true,
    env: toastEnvironment,
  });
  toast.unref();
  return toast;
}
