import fs from "fs";
import http from "http";
import path from "path";
import { exec, spawn } from "child_process";
import { fileURLToPath } from "url";
import { chromium } from "playwright";
import {
  attendanceUrl,
  COMPANY_NAME,
  CDP_CONNECT_TIMEOUT_MS,
  CHECK_INTERVAL_MS,
  CLOCK_IN_CONTROL_TIMEOUT_MS,
  CLOCK_OUT_CONTROL_TIMEOUT_MS,
  DEBUG_HOST,
  DEBUG_PORT,
  MANUAL_ATTENTION_INTERVAL_MS,
  CHROME_PROFILE_DIRECTORY,
  DISABLE_CHROME_BACKGROUND_SERVICES,
  DISABLE_ALL_UI,
  CHROME_VISIBLE,
  CLOCK_IN_MODE,
  SHOW_LOGGED_DATE,
  SHOW_TOAST_UI,
  SKIP_CHECK_FROM,
  SKIP_CHECK_UNTIL,
} from "./constants.js";
import { isLoggedToday, markLoggedToday } from "./attendanceStore.js";
import {
  focusChrome,
  getChromeDebugProfile,
  getChromePath,
  spawnToast,
  stopChromeForAttendance,
} from "./platform.js";

const __filename = fileURLToPath(import.meta.url);
const BASE = process.env.ATTENDANCE_BASE_DIR || path.dirname(path.dirname(__filename));

const DATA = process.env.ATTENDANCE_DATA_DIR || path.join(BASE, "data");
fs.mkdirSync(DATA, { recursive: true });
const lockFile = path.join(DATA, "attendance_lock.txt");
const logFile = path.join(DATA, "attendance_log.txt");
const statusFile = path.join(DATA, "toast_status.txt");
let stoppingChrome = false;

function log(message) {
  const line = `[${new Date().toLocaleString()}] ${message}\n`;
  try {
    fs.appendFileSync(logFile, line);
  } catch (err) {
    console.error(`Failed to write attendance log: ${err.message}`);
  }
  console.log(message);
}

function localDateKey(date = new Date()) {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");

  return `${year}-${month}-${day}`;
}

function isValidCompanyName(name) {
  return /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/i.test(name) &&
    !["example", "company", "your-company", "test"].includes(name.toLowerCase());
}

function timeToMinutes(value) {
  const [hours, minutes] = value.split(":").map(Number);
  return hours * 60 + minutes;
}

function quietWindowDelay(now = new Date()) {
  const current = now.getHours() * 60 + now.getMinutes();
  const start = timeToMinutes(SKIP_CHECK_FROM);
  const end = timeToMinutes(SKIP_CHECK_UNTIL);
  const inWindow = start < end
    ? current >= start && current < end
    : current >= start || current < end;
  if (!inWindow) return 0;

  let minutes = end - current;
  if (minutes <= 0) minutes += 24 * 60;
  return minutes * 60_000 - now.getSeconds() * 1_000 - now.getMilliseconds();
}

function setStatus(status) {
  const tmpStatusFile = `${statusFile}.tmp`;
  const statusText = status === "in" ? `${status} ${localDateKey()}` : status;
  fs.writeFileSync(tmpStatusFile, statusText);
  fs.renameSync(tmpStatusFile, statusFile);
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function sleepUntilNextCheck(delay, previousDate) {
  const scheduledWake = Date.now() + delay;
  await sleep(delay);

  const drift = Date.now() - scheduledWake;
  const currentDate = localDateKey();
  if (drift > 60_000) {
    log(
      `System sleep/wake detected; check resumed ${Math.round(drift / 1000)} seconds late`,
    );
  }
  if (currentDate !== previousDate) {
    setStatus("out");
    log(`Date changed from ${previousDate} to ${currentDate}; refreshing attendance`);
  }
}

function isProcessRunning(pid) {
  try {
    process.kill(pid, 0);
    return true;
  } catch {
    return false;
  }
}

function acquireLock() {
  if (fs.existsSync(lockFile)) {
    const oldPid = Number(fs.readFileSync(lockFile, "utf8").trim());
    if (oldPid && oldPid !== process.pid && isProcessRunning(oldPid)) {
      log(`Another attendance process is already running: ${oldPid}`);
      process.exit(0);
    }

    log(
      oldPid
        ? `Removing stale attendance lock for stopped process: ${oldPid}`
        : "Removing invalid attendance lock",
    );
  }

  fs.writeFileSync(lockFile, String(process.pid));
}

function releaseLock() {
  if (
    fs.existsSync(lockFile) &&
    fs.readFileSync(lockFile, "utf8").trim() === String(process.pid)
  ) {
    fs.unlinkSync(lockFile);
  }
}

function run(command) {
  return new Promise((resolve) => {
    exec(command, () => resolve());
  });
}

async function bringChromeToFront() {
  await focusChrome();
}

function canReachChromeDebugger() {
  return new Promise((resolve) => {
    const req = http.get(
      `http://${DEBUG_HOST}:${DEBUG_PORT}/json/version`,
      (res) => {
        res.resume();
        res.on("end", () => resolve(true));
      },
    );
    req.setTimeout(1000, () => {
      req.destroy();
      resolve(false);
    });
    req.on("error", () => resolve(false));
  });
}

async function firstVisibleControl(controls, timeout) {
  const startedAt = Date.now();

  while (Date.now() - startedAt < timeout) {
    for (const control of controls) {
      try {
        if (await control.locator.isVisible()) return control.name;
      } catch {
        // Keep polling while the page settles.
      }
    }

    await sleep(500);
  }

  return null;
}

async function clickFresh(locatorFactory, name, timeout = 30_000) {
  const startedAt = Date.now();
  let lastError = null;

  while (Date.now() - startedAt < timeout) {
    const locator = locatorFactory();

    try {
      await locator.waitFor({ state: "visible", timeout: 5_000 });
      await locator.click({ timeout: 5_000 });
      return;
    } catch (err) {
      lastError = err;
      if (!/detached|Timeout|not visible|not enabled/i.test(err.message))
        throw err;
      await sleep(500);
    }
  }

  throw new Error(
    `Could not click ${name}: ${lastError?.message || "timed out"}`,
  );
}

function isRetriableNavigationError(err) {
  return /ERR_INTERNET_DISCONNECTED|ERR_NETWORK_CHANGED|ERR_NAME_NOT_RESOLVED|ERR_CONNECTION_RESET|ERR_CONNECTION_TIMED_OUT|Timeout/i.test(
    err.message,
  );
}

async function gotoWithNetworkRetry(page, url, timeout = 180_000) {
  const startedAt = Date.now();
  let attempt = 0;
  let lastError = null;

  while (Date.now() - startedAt < timeout) {
    attempt += 1;

    try {
      await page.goto(url, { waitUntil: "domcontentloaded", timeout: 45_000 });
      return;
    } catch (err) {
      lastError = err;
      if (!isRetriableNavigationError(err)) throw err;

      log(
        `Navigation failed (${err.message.split("\n")[0]}). Retrying after network reconnects, attempt ${attempt}`,
      );
      await sleep(Math.min(5_000 * attempt, 30_000));
    }
  }

  throw new Error(
    `Could not load attendance page after network retries: ${lastError?.message || "timed out"}`,
  );
}

function startToast() {
  if (DISABLE_ALL_UI || (!SHOW_TOAST_UI && !SHOW_LOGGED_DATE)) {
    log("Toast process disabled by config.txt");
    return;
  }

  if (!fs.existsSync(statusFile)) {
    setStatus("out");
    log(`Created toast status file: ${statusFile}`);
  }

  log(`Starting toast UI; status file: ${statusFile}`);
  const toast = spawnToast(BASE, statusFile);
  if (!toast) return;
  toast.on("error", (err) => log(`Toast process failed to start: ${err.message}`));
  toast.on("exit", (code, signal) => {
    if (code !== 0 && code !== null) log(`Toast process exited: code=${code}`);
    else if (signal) log(`Toast process exited by signal: ${signal}`);
  });
}

async function waitForChrome(chrome, timeout = 60_000) {
  const startedAt = Date.now();
  let attempts = 0;

  while (true) {
    if (await canReachChromeDebugger()) {
      log("Chrome debugger ready");
      return;
    }

    if (chrome.exitCode !== null) {
      throw new Error(
        `Chrome exited before debugger started; exit code ${chrome.exitCode}`,
      );
    }

    if (Date.now() - startedAt >= timeout) {
      throw new Error(
        `Timed out waiting for Chrome debugger on ${DEBUG_HOST}:${DEBUG_PORT}`,
      );
    }

    attempts += 1;
    if (attempts % 10 === 0) {
      log(`Still waiting for Chrome debugger on ${DEBUG_HOST}:${DEBUG_PORT}`);
    }
    await sleep(1000);
  }
}

async function waitForChromeDebuggerToClose(timeout = 15_000) {
  const startedAt = Date.now();

  while (Date.now() - startedAt < timeout) {
    if (!(await canReachChromeDebugger())) return true;
    await sleep(500);
  }

  return false;
}

async function stopChrome() {
  stoppingChrome = true;
  try {
    await stopChromeForAttendance(getChromeDebugProfile(BASE));

    if (!(await waitForChromeDebuggerToClose())) {
      log(
        `Attendance Chrome debugger on ${DEBUG_HOST}:${DEBUG_PORT} did not close cleanly before restart`,
      );
    }
  } finally {
    stoppingChrome = false;
  }
}

async function closeBrowser(browser) {
  if (!browser) return;

  try {
    await browser.close();
  } catch (err) {
    log("Browser close failed: " + err.message);
  }
}

async function resetAutomationState(browser, reason) {
  if (reason) log(`Resetting Chrome automation state: ${reason}`);
  await closeBrowser(browser);
  await stopChrome();
}

async function connectToChrome() {
  try {
    return await chromium.connectOverCDP(`http://${DEBUG_HOST}:${DEBUG_PORT}`, {
      timeout: CDP_CONNECT_TIMEOUT_MS,
    });
  } catch (err) {
    log(`Chrome CDP connection failed, restarting once: ${err.message}`);
    await startChrome({ visible: CHROME_VISIBLE });
    return chromium.connectOverCDP(`http://${DEBUG_HOST}:${DEBUG_PORT}`, {
      timeout: CDP_CONNECT_TIMEOUT_MS,
    });
  }
}

async function startChrome({ visible = false } = {}) {
  log(
    visible
      ? "Opening Chrome for manual attention..."
      : "Restarting background Chrome...",
  );
  await stopChrome();

  const chromePath = getChromePath();
  log(`Chrome executable: ${chromePath}`);
  log(`Chrome profile: ${getChromeDebugProfile(BASE)}`);
  if (!fs.existsSync(chromePath)) {
    throw new Error(`Chrome executable not found at ${chromePath}`);
  }

  const args = [
    `--remote-debugging-port=${DEBUG_PORT}`,
    `--remote-debugging-address=${DEBUG_HOST}`,
    `--user-data-dir=${getChromeDebugProfile(BASE)}`,
    `--profile-directory=${CHROME_PROFILE_DIRECTORY}`,
    "--no-first-run",
    attendanceUrl,
  ];

  if (!visible) args.splice(args.length - 1, 0, "--headless=new");
  if (!visible && DISABLE_CHROME_BACKGROUND_SERVICES) {
    args.splice(args.length - 1, 0,
      "--disable-background-networking",
      "--disable-component-update",
      "--disable-default-apps",
      "--disable-extensions",
      "--disable-sync",
    );
  }

  const chrome = spawn(chromePath, args, {
    detached: true,
    stdio: visible ? "ignore" : ["ignore", "pipe", "pipe"],
    windowsHide: true,
  });

  chrome.unref();
  chrome.on("error", (err) => log("Chrome failed to start: " + err.message));
  if (!visible) {
    chrome.stdout.on("data", (data) =>
      log("Chrome stdout: " + data.toString().trim()),
    );
    chrome.stderr.on("data", (data) =>
      log("Chrome stderr: " + data.toString().trim()),
    );
    chrome.on("exit", (code, signal) => {
      if (stoppingChrome) {
        log("Chrome automation window closed");
      } else if (code !== null || signal) {
        log(
          `Chrome exited unexpectedly: code=${code ?? "null"} signal=${signal ?? "null"}`,
        );
      }
    });
  }

  log("Waiting for Chrome debugger port...");
  await waitForChrome(chrome);
}

async function openChromeForManualAttention() {
  log("Opening automation Chrome profile for manual attention...");

  const chromePath = getChromePath();
  log(`Manual Chrome executable: ${chromePath}`);
  log(`Manual Chrome profile: ${getChromeDebugProfile(BASE)}`);
  if (!fs.existsSync(chromePath)) {
    throw new Error(`Chrome executable not found at ${chromePath}`);
  }

  const chrome = spawn(chromePath, [
    `--user-data-dir=${getChromeDebugProfile(BASE)}`,
    `--profile-directory=${CHROME_PROFILE_DIRECTORY}`,
    "--new-window",
    "--disable-background-mode",
    "--no-first-run",
    attendanceUrl,
  ], {
    detached: true,
    stdio: "ignore",
    windowsHide: false,
  });

  chrome.unref();
  log(`Manual Chrome launch requested (pid ${chrome.pid ?? "unknown"})`);
  chrome.on("error", (err) => log("Chrome failed to start: " + err.message));
  chrome.on("exit", (code, signal) => {
    if (code !== 0 && code !== null) {
      log(`Manual Chrome process exited: code=${code}`);
    } else if (signal) {
      log(`Manual Chrome process exited by signal: ${signal}`);
    }
  });
  await sleep(1000);
  await bringChromeToFront();
}

async function clockInIfNeeded() {
  let browser = null;
  let chromeStarted = false;
  let keepChromeOpen = false;

  const quietDelay = quietWindowDelay();
  if (quietDelay > 0) {
    setStatus("out");
    log(`Skipping attendance check during quiet window ${SKIP_CHECK_FROM}-${SKIP_CHECK_UNTIL}; next check in ${Math.ceil(quietDelay / 60_000)} minutes`);
    return quietDelay;
  }

  if (isLoggedToday()) {
    setStatus("in");
    log(`Already logged for ${localDateKey()} -> sleeping`);
    return CHECK_INTERVAL_MS;
  }

  try {
    log(`Starting attendance check for ${localDateKey()}`);
    setStatus("run");
    await startChrome({ visible: CHROME_VISIBLE });
    chromeStarted = true;

    browser = await connectToChrome();
    const context = browser.contexts()[0] || (await browser.newContext());
    await context.grantPermissions(["geolocation"], { origin: attendanceUrl });

    const page = context.pages()[0] || (await context.newPage());
    log(`Navigating to attendance page: ${attendanceUrl}`);
    await gotoWithNetworkRetry(page, attendanceUrl);
    log("Attendance page loaded");

    const remoteClockIn = () => page.getByText(/Remote Clock\s*-?\s*In/i).first();
    const webClockIn = () => page.getByText(/Web Clock\s*-?\s*In/i).first();
    const submit = () =>
      page
        .locator(
          "//button[contains(@class, 'btn-primary') and contains(@class, 'btn-sm')]",
        )
        .first();
    const remoteClockOut = () => page.getByText(/Remote Clock\s*-?\s*Out/i).first();
    const webClockOut = () => page.getByText(/Web Clock\s*-?\s*Out/i).first();
    const clockOut = () => page.getByText(/(?:Remote|Web) Clock\s*-?\s*Out/i).first();
    const clockInControls = CLOCK_IN_MODE === "web"
      ? [{ name: "webClockIn", locator: webClockIn(), factory: webClockIn }]
      : CLOCK_IN_MODE === "auto"
        ? [
            { name: "remoteClockIn", locator: remoteClockIn(), factory: remoteClockIn },
            { name: "webClockIn", locator: webClockIn(), factory: webClockIn },
          ]
        : [{ name: "remoteClockIn", locator: remoteClockIn(), factory: remoteClockIn }];

    try {
      const clockOutControl = clockOut();
      if (!(await clockOutControl.isVisible())) {
        await clockOutControl.waitFor({
          state: "visible",
          timeout: CLOCK_OUT_CONTROL_TIMEOUT_MS,
        });
      }
      markLoggedToday();
      setStatus("in");
      log("Already clocked in");
      return CHECK_INTERVAL_MS;
    } catch {
      // If clock-out is not visible quickly, continue with the normal clock-in search.
    }

    log("Looking for clock-in or clock-out controls");
    const visibleControl = await firstVisibleControl(
      [
        { name: "clockOut", locator: clockOut() },
        ...clockInControls,
      ],
      Math.max(CLOCK_IN_CONTROL_TIMEOUT_MS, CLOCK_OUT_CONTROL_TIMEOUT_MS),
    );

    if (visibleControl === "remoteClockIn" || visibleControl === "webClockIn") {
      if (await clockOut().isVisible()) {
        markLoggedToday();
        setStatus("in");
        log("Already clocked in");
        return CHECK_INTERVAL_MS;
      }

      const selectedControl = clockInControls.find((control) => control.name === visibleControl);
      const label = visibleControl === "webClockIn" ? "Web Clock-In" : "Remote Clock-In";
      log(`Clicking ${label} control (mode: ${CLOCK_IN_MODE})`);
      await clickFresh(selectedControl.factory, label);
      await clickFresh(submit, "clock-in submit");
      markLoggedToday();
      setStatus("in");
      log("Clocked in successfully");
      return CHECK_INTERVAL_MS;
    }

    if (visibleControl === "clockOut") {
      markLoggedToday();
      setStatus("in");
      log("Already clocked in");
      return CHECK_INTERVAL_MS;
    }

    setStatus("out");
    log("No clock-in or clock-out control became visible");
    keepChromeOpen = true;
    await resetAutomationState(browser, "clock-in controls were not found");
    browser = null;
    chromeStarted = false;
    await openChromeForManualAttention();
    log(
      "Clock-in controls were not found; Chrome is open for manual login/check",
    );
    return MANUAL_ATTENTION_INTERVAL_MS;
  } catch (err) {
    log(`Attendance check error: ${err.message.split("\n")[0]}`);
    setStatus("out");
    if (!isRetriableNavigationError(err)) {
      keepChromeOpen = true;
      await resetAutomationState(browser, err.message.split("\n")[0]);
      browser = null;
      chromeStarted = false;
      await openChromeForManualAttention();
      log("Clock-in needs manual attention; Chrome is open: " + err.message);
      return MANUAL_ATTENTION_INTERVAL_MS;
    }

    await resetAutomationState(browser, err.message.split("\n")[0]);
    browser = null;
    chromeStarted = false;
    log(
      "Clock-in attempt failed; will try again on next scheduled check: " +
        err.message,
    );
    return CHECK_INTERVAL_MS;
  } finally {
    if (!keepChromeOpen) {
      await closeBrowser(browser);
      if (chromeStarted) await stopChrome();
    }
  }
}

async function main() {
  acquireLock();
  log("Script started");
  log(`Data directory: ${DATA}`);
  log(`Attendance URL: ${attendanceUrl}`);
  startToast();
  if (!isValidCompanyName(COMPANY_NAME)) {
    setStatus("error");
    log(`Invalid COMPANY_NAME '${COMPANY_NAME}'. Update data\\config.txt before restarting.`);
    log("Attendance paused because configuration is invalid; toast will remain visible.");
    while (true) await sleep(60_000);
  }
  while (true) {
    const checkDate = localDateKey();
    const delay = (await clockInIfNeeded()) || CHECK_INTERVAL_MS;
    log(
      `Waiting ${Math.round(delay / 1000)} seconds until next scheduled check`,
    );
    await sleepUntilNextCheck(delay, checkDate);
  }
}

main().catch(async (err) => {
  setStatus("error");
  log("Fatal error: " + err.message);
  await resetAutomationState(null, "fatal script error");
  releaseLock();
  process.exit(1);
});

process.on("exit", releaseLock);
process.on("SIGINT", () => process.exit(0));
process.on("SIGTERM", () => process.exit(0));
