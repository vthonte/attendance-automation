// Desktop packaging entry point. The attendance engine remains reusable outside Electron.
import path from "path";
import fs from "fs";
import { fileURLToPath } from "url";

process.env.ATTENDANCE_PACKAGED = "1";
const dataDir = process.env.PORTABLE_EXECUTABLE_DIR || path.dirname(process.execPath);
const configFile = path.join(dataDir, "config.txt");
const bundledConfig = path.join(path.dirname(fileURLToPath(import.meta.url)), "config.txt");
fs.mkdirSync(dataDir, { recursive: true });
if (!fs.existsSync(configFile) && fs.existsSync(bundledConfig)) {
  fs.copyFileSync(bundledConfig, configFile);
}
process.env.ATTENDANCE_DATA_DIR = dataDir;
process.env.ATTENDANCE_CONFIG_FILE = configFile;

if (process.argv.includes("--toast")) {
  await import("./notifyToast.js");
} else {
  await import("./attendance.js");
}
