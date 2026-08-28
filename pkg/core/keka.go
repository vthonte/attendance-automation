package core

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type CheckResult string

const (
	ResultAlreadyClockedIn CheckResult = "already_clocked_in"
	ResultClockedIn        CheckResult = "clocked_in"
	ResultNeedsAttention   CheckResult = "needs_attention"
	ResultRetryLater       CheckResult = "retry_later"
)

type jsEvaluationResult struct {
	State   string `json:"state"`
	Message string `json:"message"`
	Button  string `json:"button"`
}

const kekaInspectorScript = `
(() => {
  function isVisible(el) {
    if (!el) return false;
    const style = window.getComputedStyle(el);
    if (style.display === 'none' || style.visibility === 'hidden' || style.opacity === '0') return false;
    const rect = el.getBoundingClientRect();
    return rect.width > 0 && rect.height > 0;
  }

  function findByText(regexStr, tagFilter = 'button, a, span, div, p, strong') {
    const regex = new RegExp(regexStr, 'i');
    const elements = Array.from(document.querySelectorAll(tagFilter));
    for (const el of elements) {
      if (regex.test(el.textContent) && isVisible(el)) {
        return el;
      }
    }
    return null;
  }

  // 1. Check if already clocked out / in
  const clockOut = findByText('(?:Remote|Web)\\s*Clock\\s*-?\\s*Out');
  if (clockOut) {
    return JSON.stringify({ state: 'already_clocked_in', message: 'Clock-out button is present' });
  }

  // 2. Check for clock-in buttons
  const remoteIn = findByText('Remote\\s*Clock\\s*-?\\s*In');
  const webIn = findByText('Web\\s*Clock\\s*-?\\s*In');

  let targetBtn = null;
  let btnType = '';

  const mode = '%s'; // web, remote, auto

  if (mode === 'web') {
    if (webIn) { targetBtn = webIn; btnType = 'webClockIn'; }
  } else if (mode === 'remote') {
    if (remoteIn) { targetBtn = remoteIn; btnType = 'remoteClockIn'; }
  } else { // auto
    if (remoteIn) { targetBtn = remoteIn; btnType = 'remoteClockIn'; }
    else if (webIn) { targetBtn = webIn; btnType = 'webClockIn'; }
  }

  if (!targetBtn) {
    if (remoteIn || webIn) {
      targetBtn = remoteIn || webIn;
      btnType = remoteIn ? 'remoteClockIn' : 'webClockIn';
    }
  }

  if (targetBtn) {
    return JSON.stringify({ state: 'clock_in_found', button: btnType });
  }

  // 3. Check if on a login page or requires attention
  const isLoginPage = window.location.href.includes('/login') || 
                      document.querySelector('input[type="password"]') !== null ||
                      findByText('Sign\\s*in', 'button, a, h1, h2');
  if (isLoginPage) {
    return JSON.stringify({ state: 'needs_login', message: 'User is on login page' });
  }

  return JSON.stringify({ state: 'not_found', message: 'No attendance controls found' });
})()
`

const kekaClockInScript = `
(async () => {
  function isVisible(el) {
    if (!el) return false;
    const style = window.getComputedStyle(el);
    if (style.display === 'none' || style.visibility === 'hidden' || style.opacity === '0') return false;
    const rect = el.getBoundingClientRect();
    return rect.width > 0 && rect.height > 0;
  }

  function findByText(regexStr, tagFilter = 'button, a, span, div, p, strong') {
    const regex = new RegExp(regexStr, 'i');
    const elements = Array.from(document.querySelectorAll(tagFilter));
    for (const el of elements) {
      if (regex.test(el.textContent) && isVisible(el)) {
        return el;
      }
    }
    return null;
  }

  function sleep(ms) {
    return new Promise(r => setTimeout(r, ms));
  }

  const mode = '%s';
  const remoteIn = findByText('Remote\\s*Clock\\s*-?\\s*In');
  const webIn = findByText('Web\\s*Clock\\s*-?\\s*In');

  let targetBtn = null;
  if (mode === 'web') targetBtn = webIn || remoteIn;
  else if (mode === 'remote') targetBtn = remoteIn || webIn;
  else targetBtn = remoteIn || webIn;

  if (!targetBtn) {
    return JSON.stringify({ state: 'error', message: 'Clock-in button vanished' });
  }

  // Click clock-in
  targetBtn.click();
  await sleep(1500);

  // Look for confirm / submit button in popup modal
  let submitBtn = document.querySelector("button.btn-primary.btn-sm") ||
                  document.querySelector(".modal-footer button.btn-primary") ||
                  document.querySelector("button.btn-primary");

  if (!submitBtn) {
    submitBtn = findByText('^(?:Submit|Confirm|Clock\\s*In|Save)$', 'button');
  }

  if (submitBtn) {
    submitBtn.click();
    await sleep(2000);
    return JSON.stringify({ state: 'success', message: 'Submitted clock-in successfully' });
  }

  return JSON.stringify({ state: 'error', message: 'Submit button not found in modal' });
})()
`

func PerformKekaCheckAndClockIn(ctx context.Context, cdp *CDPClient, cfg *Config) (CheckResult, error) {
	// Grant geolocation permissions
	Log(cfg.DataDir, "Granting permissions for attendance URL...")
	_ = cdp.GrantPermissions(ctx, cfg.AttendanceURL, []string{"geolocation"})

	// Navigate with retry
	Log(cfg.DataDir, fmt.Sprintf("Navigating to attendance page: %s", cfg.AttendanceURL))
	if err := cdp.Navigate(ctx, cfg.AttendanceURL); err != nil {
		return ResultRetryLater, fmt.Errorf("navigation failed: %w", err)
	}

	Log(cfg.DataDir, "Attendance page loaded. Inspecting controls...")

	deadline := time.Now().Add(cfg.ClockInControlTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ResultRetryLater, ctx.Err()
		default:
		}

		script := fmt.Sprintf(kekaInspectorScript, string(cfg.ClockInMode))
		val, err := cdp.Evaluate(ctx, script)
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}

		var rawStr string
		_ = json.Unmarshal(val, &rawStr)
		if rawStr == "" {
			rawStr = string(val)
		}

		var res jsEvaluationResult
		if err := json.Unmarshal([]byte(rawStr), &res); err != nil {
			time.Sleep(1 * time.Second)
			continue
		}

		switch res.State {
		case "already_clocked_in":
			Log(cfg.DataDir, "Already clocked in (detected clock-out control)")
			return ResultAlreadyClockedIn, nil

		case "clock_in_found":
			Log(cfg.DataDir, fmt.Sprintf("Found %s control (mode: %s). Attempting clock-in...", res.Button, cfg.ClockInMode))
			clickScript := fmt.Sprintf(kekaClockInScript, string(cfg.ClockInMode))
			clickVal, err := cdp.Evaluate(ctx, clickScript)
			if err != nil {
				return ResultNeedsAttention, fmt.Errorf("failed executing clock-in click: %w", err)
			}

			var clickRawStr string
			_ = json.Unmarshal(clickVal, &clickRawStr)
			if clickRawStr == "" {
				clickRawStr = string(clickVal)
			}

			var clickRes jsEvaluationResult
			_ = json.Unmarshal([]byte(clickRawStr), &clickRes)
			if clickRes.State == "success" {
				Log(cfg.DataDir, "Clocked in successfully!")
				return ResultClockedIn, nil
			}
			Log(cfg.DataDir, fmt.Sprintf("Clock-in submission status: %s (%s)", clickRes.State, clickRes.Message))
			return ResultNeedsAttention, nil

		case "needs_login":
			Log(cfg.DataDir, "Keka requires login/authentication.")
			return ResultNeedsAttention, nil
		}

		time.Sleep(1 * time.Second)
	}

	Log(cfg.DataDir, "Clock-in controls were not found within timeout.")
	return ResultNeedsAttention, nil
}
