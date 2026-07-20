import { app, BrowserWindow, screen } from "electron";
import path from "path";
import fs from "fs";
import chokidar from "chokidar";
import { fileURLToPath } from "url";
import { config } from "./configLoader.js";

app.disableHardwareAcceleration();

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const toastArgumentIndex = process.argv.indexOf("--toast");
const dataDir = process.env.ATTENDANCE_DATA_DIR || __dirname;
const statusFile = (toastArgumentIndex >= 0 && process.argv[toastArgumentIndex + 1]) || process.argv[2] || path.join(dataDir, "toast_status.txt");
const storeFile = path.join(dataDir, "attendance_store.json");
const logFile = path.join(dataDir, "toast_log.txt");

let win;
let lastStatus = "out";
let lastLoggedDate = "";

function log(message) {
  const timestamp = new Date().toISOString();
  const line = `[${timestamp}] ${message}\n`;
  fs.appendFileSync(logFile, line);
  console.log(line.trim());
}

function formatDisplayDate(dateKey) {
  if (!dateKey) return "";

  const [year, month, day] = dateKey.split("-").map(Number);
  if (!year || !month || !day) return dateKey;

  return new Date(year, month - 1, day).toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
  });
}

function localDateKey(date = new Date()) {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function isStoredForToday() {
  try {
    const store = JSON.parse(fs.readFileSync(storeFile, "utf-8"));
    return store[localDateKey()] === true;
  } catch (err) {
    log(`ERROR reading attendance store: ${err.message}`);
    return false;
  }
}

function updateToastFromFile() {
  try {
    if (!fs.existsSync(statusFile)) {
      fs.writeFileSync(statusFile, "out");
      log(`Status file not found. Created new file with 'out'.`);
    }

    const [rawStatus, rawDate] = fs
      .readFileSync(statusFile, "utf-8")
      .trim()
      .split(/\s+/);
    const content = rawStatus || lastStatus;
    const loggedDate = rawDate || lastLoggedDate;
    const today = localDateKey();
    const storedToday = isStoredForToday();
    const loggedToday = storedToday || (content === "in" && loggedDate === today);
    let color = "lightgray";
    let text = "";

    const displayDate = storedToday ? today : loggedDate;

    switch (loggedToday ? "in" : content === "in" ? "out" : content) {
      case "run":
        color = "khaki";
        text = "Checking";
        break;
      case "in":
        color = "lightgreen";
        text = displayDate ? `Logged ${formatDisplayDate(displayDate)}` : "Logged";
        break;
      case "error":
        color = "lightcoral";
        text = "Needs attention";
        break;
      case "out":
        color = "lightgray";
        text = "Not logged";
        break;
      default:
        color = "lightgray";
        text = content;
        break;
    }

    if (rawStatus) lastStatus = content;
    if (rawDate) lastLoggedDate = rawDate;

    if (win) {
      win.webContents.send("update-toast", {
        text: config.showLoggedDate ? text : "",
        color,
        barVisible: config.showToastUi,
      });
      log(`Updated toast: text='${text}', color='${color}'`);
    }
  } catch (err) {
    log(`ERROR reading file: ${err.message}`);
  }
}

function watchFile() {
  updateToastFromFile();

  chokidar.watch(statusFile).on("change", () => {
    updateToastFromFile();
  });
  chokidar.watch(storeFile).on("change", () => {
    updateToastFromFile();
  });

  // Re-check at midnight even when neither watched file changes.
  setInterval(updateToastFromFile, 60_000);

  log("File watcher initialized.");
}

function createWindow() {
  const { width, height } = screen.getPrimaryDisplay().workAreaSize;
  log("Creating toast window...");
  win = new BrowserWindow({
    width,
    height: config.toastHeight,
    x: 0,
    y: 0,
    show: true,
    frame: false,
    opacity: 1,
    alwaysOnTop: true,
    backgroundColor: "#00000000",
    transparent: true,
    skipTaskbar: true,
    focusable: false,
    webPreferences: {
      nodeIntegration: true,
      contextIsolation: false,
    },
  });

  win.loadFile(path.join(__dirname, "toast.html"));

  win.once("ready-to-show", () => {
    win.showInactive();
    win.setAlwaysOnTop(true, "screen-saver");
    win.setIgnoreMouseEvents(true, { forward: true });
  });
  win.setAlwaysOnTop(true, "screen-saver");

  log(`Toast window created. Watching file: ${statusFile}`);
  watchFile();
}

app.whenReady().then(createWindow);

app.on("window-all-closed", () => {
  log("All windows closed. Exiting.");
  app.quit();
});
