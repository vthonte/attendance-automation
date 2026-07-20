import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const base = path.dirname(fileURLToPath(import.meta.url));
const file = process.env.ATTENDANCE_CONFIG_FILE || path.join(base, "config.txt");
const values = {};

if (fs.existsSync(file)) {
  for (const line of fs.readFileSync(file, "utf8").split(/\r?\n/)) {
    const text = line.trim();
    if (!text || text.startsWith("#") || !text.includes("=")) continue;
    const index = text.indexOf("=");
    values[text.slice(0, index).trim()] = text.slice(index + 1).trim();
  }
}

const number = (key, fallback) => Number(values[key] ?? fallback);
const boolean = (key, fallback) => {
  if (values[key] === undefined) return fallback;
  return !["false", "0", "no", "off"].includes(values[key].toLowerCase());
};
export const config = {
  attendanceUrl: values.ATTENDANCE_URL,
  checkIntervalMs: number("CHECK_INTERVAL_MS", 60000),
  manualAttentionIntervalMs: number("MANUAL_ATTENTION_INTERVAL_MS", 150000),
  clockInControlTimeoutMs: number("CLOCK_IN_CONTROL_TIMEOUT_MS", 30000),
  clockOutControlTimeoutMs: number("CLOCK_OUT_CONTROL_TIMEOUT_MS", 15000),
  cdpConnectTimeoutMs: number("CDP_CONNECT_TIMEOUT_MS", 120000),
  debugHost: values.DEBUG_HOST || "127.0.0.1",
  debugPort: number("DEBUG_PORT", 9222),
  chromeProfileDirectory: values.CHROME_PROFILE_DIRECTORY || "Default",
  showToastUi: boolean("SHOW_TOAST_UI", true),
  showLoggedDate: boolean("SHOW_LOGGED_DATE", true),
  toastHeight: number("TOAST_HEIGHT", 32),
  disableChromeBackgroundServices: boolean("DISABLE_CHROME_BACKGROUND_SERVICES", true),
  disableAllUi: boolean("DISABLE_ALL_UI", false),
};
