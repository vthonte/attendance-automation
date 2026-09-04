package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
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

  function findActionLink(regexStr) {
    const regex = new RegExp(regexStr, 'i');
    // Only search clickable action elements (a, button), NOT table history rows or containers
    const candidates = Array.from(document.querySelectorAll('a, button, [role="button"]'));
    for (const el of candidates) {
      if (el.closest('table, tbody, tr, .attendance-logs-table, .logs-table, .history-table')) continue;
      const text = (el.innerText || el.textContent || '').trim();
      if (regex.test(text) && isVisible(el) && text.length < 50) {
        return el;
      }
    }
    return null;
  }

  const currentHref = window.location.href;

  // 1. Check if user is on a login / authentication page
  const isLoginPage = currentHref.includes('/login') || 
                      currentHref.includes('login.microsoftonline.com') ||
                      currentHref.includes('accounts.google.com') ||
                      currentHref.includes('auth0.com') ||
                      currentHref.includes('okta.com') ||
                      document.querySelector('input[type="password"]') !== null;
  if (isLoginPage) {
    return JSON.stringify({ state: 'needs_login', message: 'User is on login page' });
  }

  // 2. If authenticated but not on attendance logs page, redirect to logs page
  const attendanceUrl = '%s';
  if (!currentHref.includes('/me/attendance/logs')) {
    if (window.location.hostname.includes('keka.com') && window.location.hash) {
      window.location.hash = '#/me/attendance/logs';
    } else {
      window.location.href = attendanceUrl;
    }
    return JSON.stringify({ state: 'redirecting', message: 'Redirecting to attendance logs page' });
  }

  // 3. PRIORITY 1: CHECK FOR CLOCK-IN BUTTONS FIRST!
  // If Web Clock-In or Remote Clock-In is visible, the user is NOT clocked in.
  const webIn = findActionLink('Web\\s*Clock\\s*-?\\s*In');
  const remoteIn = findActionLink('Remote\\s*Clock\\s*-?\\s*In');
  const genericIn = findActionLink('^Clock\\s*-?\\s*In$');

  const mode = '%s'; // web, remote, auto
  let targetBtn = null;
  let btnType = '';

  if (mode === 'web') {
    if (webIn) { targetBtn = webIn; btnType = 'webClockIn'; }
    else if (genericIn) { targetBtn = genericIn; btnType = 'webClockIn'; }
    else if (remoteIn) { targetBtn = remoteIn; btnType = 'remoteClockIn'; }
  } else if (mode === 'remote') {
    if (remoteIn) { targetBtn = remoteIn; btnType = 'remoteClockIn'; }
    else if (genericIn) { targetBtn = genericIn; btnType = 'remoteClockIn'; }
    else if (webIn) { targetBtn = webIn; btnType = 'webClockIn'; }
  } else { // auto
    if (remoteIn) { targetBtn = remoteIn; btnType = 'remoteClockIn'; }
    else if (webIn) { targetBtn = webIn; btnType = 'webClockIn'; }
    else if (genericIn) { targetBtn = genericIn; btnType = 'genericClockIn'; }
  }

  if (targetBtn) {
    return JSON.stringify({ state: 'clock_in_found', button: btnType });
  }

  // 4. PRIORITY 2: ONLY IF NO CLOCK-IN BUTTON IS FOUND, CHECK FOR CLOCK-OUT
  // When clocked in, Keka displays a red button:
  // <button class="btn btn-danger mb-8">Web Clock-out</button> or <button class="btn btn-danger mb-8">Remote Clock-out</button>
  const dangerBtn = document.querySelector('button.btn-danger');
  const hasDangerClockOut = dangerBtn && isVisible(dangerBtn) && /Clock\s*-?\s*out/i.test((dangerBtn.innerText || dangerBtn.textContent || '').trim());
  const webOut = findActionLink('Web\\s*Clock\\s*-?\\s*Out');
  const remoteOut = findActionLink('Remote\\s*Clock\\s*-?\\s*Out');
  const genericOut = findActionLink('^Clock\\s*-?\\s*Out$');

  if (hasDangerClockOut || webOut || remoteOut || genericOut) {
    return JSON.stringify({ state: 'already_clocked_in', message: 'Clock-out button is present in action widget' });
  }

  return JSON.stringify({ state: 'not_found', message: 'No attendance controls found' });
})()
`

const kekaGeolocationOverrideScript = `
(() => {
  try {
    const mockPos = {
      coords: {
        latitude: 12.9716,
        longitude: 77.5946,
        accuracy: 25,
        altitude: null,
        altitudeAccuracy: null,
        heading: null,
        speed: null
      },
      timestamp: Date.now()
    };

    if (navigator.geolocation) {
      navigator.geolocation.getCurrentPosition = function(success, error, options) {
        if (typeof success === 'function') setTimeout(() => success(mockPos), 50);
      };
      navigator.geolocation.watchPosition = function(success, error, options) {
        if (typeof success === 'function') setTimeout(() => success(mockPos), 50);
        return 1;
      };
    }

    if (navigator.permissions && navigator.permissions.query) {
      const origQuery = navigator.permissions.query.bind(navigator.permissions);
      navigator.permissions.query = function(params) {
        if (params && params.name === 'geolocation') {
          return Promise.resolve({
            state: 'granted',
            name: 'geolocation',
            onchange: null
          });
        }
        return origQuery(params);
      };
    }
  } catch(e) {}
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

  function findActionLink(regexStr) {
    const regex = new RegExp(regexStr, 'i');
    const candidates = Array.from(document.querySelectorAll('a, button, [role="button"]'));
    for (const el of candidates) {
      if (el.closest('table, tbody, tr, .attendance-logs-table, .logs-table, .history-table')) continue;
      const text = (el.innerText || el.textContent || '').trim();
      if (regex.test(text) && isVisible(el) && text.length < 50) {
        return el;
      }
    }
    return null;
  }

  function clickElement(el) {
    el.scrollIntoView({ behavior: 'instant', block: 'center' });
    el.focus();
    el.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, cancelable: true, view: window }));
    el.dispatchEvent(new MouseEvent('mouseup', { bubbles: true, cancelable: true, view: window }));
    el.click();
  }

  function sleep(ms) {
    return new Promise(r => setTimeout(r, ms));
  }

  // 1. Ensure Geolocation and permissions query mocks are active in JS context
  try {
    const mockPos = {
      coords: {
        latitude: 12.9716,
        longitude: 77.5946,
        accuracy: 25,
        altitude: null,
        altitudeAccuracy: null,
        heading: null,
        speed: null
      },
      timestamp: Date.now()
    };

    if (navigator.geolocation) {
      navigator.geolocation.getCurrentPosition = function(success) {
        if (typeof success === 'function') setTimeout(() => success(mockPos), 50);
      };
      navigator.geolocation.watchPosition = function(success) {
        if (typeof success === 'function') setTimeout(() => success(mockPos), 50);
        return 1;
      };
    }

    if (navigator.permissions && navigator.permissions.query) {
      const origQuery = navigator.permissions.query.bind(navigator.permissions);
      navigator.permissions.query = function(params) {
        if (params && params.name === 'geolocation') {
          return Promise.resolve({
            state: 'granted',
            name: 'geolocation',
            onchange: null
          });
        }
        return origQuery(params);
      };
    }
  } catch(e) {}

  const mode = '%s';
  const webIn = findActionLink('Web\\s*Clock\\s*-?\\s*In');
  const remoteIn = findActionLink('Remote\\s*Clock\\s*-?\\s*In');
  const genericIn = findActionLink('^Clock\\s*-?\\s*In$');

  let targetBtn = null;
  if (mode === 'web') targetBtn = webIn || genericIn || remoteIn;
  else if (mode === 'remote') targetBtn = remoteIn || genericIn || webIn;
  else targetBtn = remoteIn || webIn || genericIn;

  if (!targetBtn) {
    return JSON.stringify({ state: 'error', message: 'Clock-in button vanished' });
  }

  // Click initial clock-in link/button with full event sequence
  clickElement(targetBtn);

  // 2. Poll for modal / submit button and verify clock-out transition up to 20 seconds
  const start = Date.now();
  let submitClicked = false;

  while (Date.now() - start < 20000) {
    await sleep(500);

    // Check if clocked out button (btn-danger) has appeared!
    const dangerBtn = document.querySelector('button.btn-danger');
    const hasDanger = dangerBtn && isVisible(dangerBtn) && /Clock\s*-?\s*out/i.test((dangerBtn.innerText || dangerBtn.textContent || '').trim());
    const clockOutLink = findActionLink('(?:Remote|Web)?\\s*Clock\\s*-?\\s*Out');
    if (hasDanger || clockOutLink) {
      return JSON.stringify({ state: 'success', message: 'Clocked in successfully (Clock-Out is now visible)' });
    }

    // Check for submit button (matching no-build: button.btn-primary.btn-sm)
    if (!submitClicked) {
      const submitCandidates = Array.from(document.querySelectorAll('button.btn-primary.btn-sm, button.btn-primary, [type="submit"], .modal button.btn-primary, ngb-modal-window button.btn-primary, .modal button'));
      const visibleSubmit = submitCandidates.find(b =>
        isVisible(b) &&
        !/^(?:Cancel|Close|Dismiss|No|Back)$/i.test((b.innerText || b.textContent || '').trim()) &&
        (b.classList.contains('btn-primary') || b.getAttribute('type') === 'submit' || /^(?:Confirm|Submit|Save|Proceed|Yes|OK|Clock\\s*-?\\s*In|Web\\s*Clock\\s*-?\\s*In|Remote\\s*Clock\\s*-?\\s*In|Punch\\s*In)/i.test((b.innerText || b.textContent || '').trim()))
      );

      if (visibleSubmit) {
        submitClicked = true;
        clickElement(visibleSubmit);
        await sleep(1500);
      }
    }
  }

  // Final check
  const dangerBtnFinal = document.querySelector('button.btn-danger');
  const hasDangerFinal = dangerBtnFinal && isVisible(dangerBtnFinal) && /Clock\s*-?\s*out/i.test((dangerBtnFinal.innerText || dangerBtnFinal.textContent || '').trim());
  if (hasDangerFinal) {
    return JSON.stringify({ state: 'success', message: 'Clocked in successfully (Clock-Out is now visible)' });
  }

  return JSON.stringify({ state: 'error', message: 'Clock-out button did not appear within 20 seconds after clicking clock-in' });
})()
`

func PerformKekaCheckAndClockIn(ctx context.Context, cdp *CDPClient, cfg *Config) (CheckResult, error) {
	// Parse clean origin for Browser.grantPermissions
	cleanOrigin := cfg.AttendanceURL
	if u, err := url.Parse(cfg.AttendanceURL); err == nil && u.Scheme != "" && u.Host != "" {
		cleanOrigin = fmt.Sprintf("%s://%s", u.Scheme, u.Host)
	}

	Log(cfg.DataDir, fmt.Sprintf("Granting geolocation permissions for %s...", cleanOrigin))
	_ = cdp.GrantPermissions(ctx, cleanOrigin, []string{"geolocation"})
	_ = cdp.SetGeolocationOverride(ctx, 12.9716, 77.5946, 25.0)

	// Navigate with retry
	Log(cfg.DataDir, fmt.Sprintf("Navigating to attendance page: %s", cfg.AttendanceURL))
	if err := cdp.Navigate(ctx, cfg.AttendanceURL); err != nil {
		return ResultRetryLater, fmt.Errorf("navigation failed: %w", err)
	}

	_ = cdp.GrantPermissions(ctx, cleanOrigin, []string{"geolocation"})
	_ = cdp.SetGeolocationOverride(ctx, 12.9716, 77.5946, 25.0)
	_, _ = cdp.Evaluate(ctx, kekaGeolocationOverrideScript)

	Log(cfg.DataDir, "Attendance page loaded. Inspecting controls...")

	deadline := time.Now().Add(cfg.ClockInControlTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ResultRetryLater, ctx.Err()
		default:
		}

		script := fmt.Sprintf(kekaInspectorScript, cfg.AttendanceURL, string(cfg.ClockInMode))
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

		case "redirecting":
			Log(cfg.DataDir, "User logged in! Redirecting browser to attendance logs page...")
			time.Sleep(2 * time.Second)
			continue

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
