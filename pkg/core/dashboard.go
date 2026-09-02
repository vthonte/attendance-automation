package core

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const webDashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Attendance Automation Dashboard</title>
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #0b0f17; color: #e1e7ec; padding: 28px 16px; min-height: 100vh; }
    .container { max-width: 760px; margin: 0 auto; }
    .card { background: #131b26; border: 1px solid #222f42; border-radius: 14px; padding: 24px; margin-bottom: 20px; box-shadow: 0 8px 32px rgba(0,0,0,0.35); }
    .header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 8px; }
    h1 { font-size: 20px; font-weight: 600; display: flex; align-items: center; gap: 10px; }
    .badge { display: inline-flex; align-items: center; gap: 6px; padding: 6px 14px; border-radius: 20px; font-size: 13px; font-weight: 600; }
    .badge.in { background: rgba(144, 238, 144, 0.15); color: #86efac; border: 1px solid #4ade80; }
    .badge.run { background: rgba(240, 230, 140, 0.15); color: #fde047; border: 1px solid #eab308; }
    .badge.error { background: rgba(240, 128, 128, 0.15); color: #fca5a5; border: 1px solid #f87171; }
    .badge.out { background: rgba(211, 211, 211, 0.15); color: #cbd5e1; border: 1px solid #94a3b8; }
    .dot { width: 8px; height: 8px; border-radius: 50%; background: currentColor; }
    .subtitle { color: #8a99ad; font-size: 13px; margin-bottom: 20px; }
    
    .info-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 12px; margin-bottom: 22px; }
    @media (max-width: 600px) { .info-grid { grid-template-columns: repeat(2, 1fr); } }
    .info-item { background: #0d131d; padding: 12px 14px; border-radius: 10px; border: 1px solid #1a2536; }
    .info-label { font-size: 11px; color: #73849a; margin-bottom: 4px; text-transform: uppercase; letter-spacing: 0.6px; font-weight: 600; }
    .info-value { font-size: 14px; font-weight: 500; color: #f1f5f9; }

    .actions { display: flex; flex-wrap: wrap; gap: 10px; align-items: center; padding-top: 6px; border-top: 1px solid #1c283a; }
    button { display: inline-flex; align-items: center; gap: 6px; border: none; padding: 9px 16px; border-radius: 8px; font-weight: 500; font-size: 13px; cursor: pointer; transition: all 0.2s; }
    button.primary { background: #2563eb; color: white; }
    button.primary:hover:not(:disabled) { background: #1d4ed8; }
    button.secondary { background: #1e293b; color: #cbd5e1; border: 1px solid #334155; }
    button.secondary:hover:not(:disabled) { background: #334155; color: white; }
    button.danger { background: rgba(239, 68, 68, 0.15); color: #fca5a5; border: 1px solid #dc2626; }
    button.danger:hover:not(:disabled) { background: #dc2626; color: white; }
    button:disabled { opacity: 0.5; cursor: not-allowed; }
    .action-msg { font-size: 12px; color: #94a3b8; margin-left: auto; }

    .logs-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 12px; }
    .logs-title { font-size: 15px; font-weight: 600; color: #cbd5e1; }
    .logs-box { background: #070a0f; border: 1px solid #1a2434; border-radius: 10px; padding: 14px; font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 12px; line-height: 1.6; max-height: 320px; overflow-y: auto; color: #94a3b8; white-space: pre-wrap; user-select: text; -webkit-user-select: text; cursor: text; }
  </style>
</head>
<body>
  <div class="container">
    <div class="card">
      <div class="header">
        <h1>
          <span>Attendance Automation</span>
        </h1>
        <span id="statusBadge" class="badge {{.StatusClass}}">
          <span class="dot"></span>
          <span id="statusText">{{.StatusText}}</span>
        </span>
      </div>
      <p class="subtitle">Keka Automated Clock-In Service & Management</p>

      <div class="info-grid">
        <div class="info-item">
          <div class="info-label">Company</div>
          <div class="info-value">{{.CompanyName}}</div>
        </div>
        <div class="info-item">
          <div class="info-label">Clock-In Mode</div>
          <div class="info-value">{{.ClockInMode}}</div>
        </div>
        <div class="info-item">
          <div class="info-label">Top Bar Line</div>
          <div class="info-value">{{.BarHeight}}px</div>
        </div>
        <div class="info-item">
          <div class="info-label">Check Interval</div>
          <div class="info-value">{{.CheckInterval}}</div>
        </div>
        <div class="info-item">
          <div class="info-label">Quiet Window</div>
          <div class="info-value">{{.SkipFrom}} - {{.SkipUntil}}</div>
        </div>
        <div class="info-item">
          <div class="info-label">Web UI Port</div>
          <div class="info-value">{{.Port}}</div>
        </div>
      </div>

      <div class="actions">
        <button id="checkBtn" class="primary" onclick="triggerCheck()">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M20 6L9 17l-5-5"/></svg>
          Check Attendance Now
        </button>
        <button id="restartBtn" class="secondary" onclick="triggerRestart()">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21.5 2v6h-6M21.34 15.57a10 10 0 1 1-.57-8.38l5.67-5.67"/></svg>
          Restart Loop
        </button>
        <button id="stopBtn" class="danger" onclick="triggerStop()">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="18" height="18" rx="2" ry="2"/></svg>
          Stop Service
        </button>
        <span id="actionMsg" class="action-msg">Ready</span>
      </div>
    </div>

    <div class="card">
      <div class="logs-header">
        <span class="logs-title">Live Service Logs</span>
        <div style="display: flex; gap: 8px;">
          <button id="copyLogsBtn" class="secondary" style="padding: 4px 12px; font-size: 11px;" onclick="copyAllLogs()">📋 Copy Logs</button>
          <button class="secondary" style="padding: 4px 10px; font-size: 11px;" onclick="updateStatus(true)">Refresh</button>
        </div>
      </div>
      <div id="logsBox" class="logs-box">{{.Logs}}</div>
    </div>
  </div>

  <script>
    async function triggerCheck() {
      const btn = document.getElementById('checkBtn');
      const msg = document.getElementById('actionMsg');
      btn.disabled = true;
      msg.textContent = 'Running attendance check...';

      try {
        await fetch('/api/check', { method: 'POST' });
        let attempts = 0;
        const interval = setInterval(async () => {
          attempts++;
          await updateStatus();
          if (attempts >= 5) {
            clearInterval(interval);
            btn.disabled = false;
            msg.textContent = 'Check finished. Loop active.';
          }
        }, 1200);
      } catch (e) {
        btn.disabled = false;
        msg.textContent = 'Failed to check.';
      }
    }

    async function triggerRestart() {
      const btn = document.getElementById('restartBtn');
      const msg = document.getElementById('actionMsg');
      btn.disabled = true;
      msg.textContent = 'Restarting check loop...';
      try {
        await fetch('/api/check', { method: 'POST' });
        setTimeout(async () => {
          await updateStatus();
          btn.disabled = false;
          msg.textContent = 'Loop restarted.';
        }, 1500);
      } catch (e) {
        btn.disabled = false;
      }
    }

    async function triggerStop() {
      if (!confirm('Are you sure you want to stop Attendance Automation?')) return;
      const msg = document.getElementById('actionMsg');
      msg.textContent = 'Stopping service...';
      try {
        await fetch('/api/stop', { method: 'POST' });
        msg.textContent = 'Service stopped cleanly.';
        const badge = document.getElementById('statusBadge');
        badge.className = 'badge out';
        document.getElementById('statusText').textContent = 'Stopped';
        document.getElementById('checkBtn').disabled = true;
        document.getElementById('restartBtn').disabled = true;
        document.getElementById('stopBtn').disabled = true;
      } catch (e) {
        msg.textContent = 'Service stopped.';
      }
    }

    function isTextSelectedIn(element) {
      const selection = window.getSelection();
      if (!selection || selection.rangeCount === 0 || selection.isCollapsed) return false;
      return element.contains(selection.anchorNode) || element.contains(selection.focusNode);
    }

    async function copyAllLogs() {
      const btn = document.getElementById('copyLogsBtn');
      const box = document.getElementById('logsBox');
      const text = box.textContent || '';
      try {
        await navigator.clipboard.writeText(text);
        btn.textContent = '✅ Copied!';
        setTimeout(() => { btn.textContent = '📋 Copy Logs'; }, 2000);
      } catch (e) {
        const textarea = document.createElement('textarea');
        textarea.value = text;
        textarea.style.position = 'fixed';
        textarea.style.opacity = '0';
        document.body.appendChild(textarea);
        textarea.select();
        document.execCommand('copy');
        document.body.removeChild(textarea);
        btn.textContent = '✅ Copied!';
        setTimeout(() => { btn.textContent = '📋 Copy Logs'; }, 2000);
      }
    }

    async function updateStatus(force = false) {
      try {
        const res = await fetch('/api/status');
        const data = await res.json();
        const badge = document.getElementById('statusBadge');
        badge.className = 'badge ' + data.status;
        document.getElementById('statusText').textContent = data.status_text;

        if (data.logs !== undefined) {
          const logsBox = document.getElementById('logsBox');
          // Only update DOM if text changed AND user is not actively selecting text
          if (logsBox.textContent !== data.logs) {
            if (force || !isTextSelectedIn(logsBox)) {
              const isScrolledToBottom = logsBox.scrollHeight - logsBox.clientHeight <= logsBox.scrollTop + 60;
              logsBox.textContent = data.logs;
              if (isScrolledToBottom) {
                logsBox.scrollTop = logsBox.scrollHeight;
              }
            }
          }
        }
      } catch (e) {}
    }

    // Scroll logs to bottom initially
    const box = document.getElementById('logsBox');
    if (box) box.scrollTop = box.scrollHeight;

    // Auto-poll status every 3 seconds
    setInterval(updateStatus, 3000);
  </script>
</body>
</html>`

type dashboardData struct {
	StatusClass   string
	StatusText    string
	CompanyName   string
	ClockInMode   string
	BarHeight     int
	SkipFrom      string
	SkipUntil     string
	CheckInterval string
	Port          int
	Logs          string
}

func OpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd.exe", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

func IsDashboardRunning(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func StartWebDashboard(ctx context.Context, engine *Engine, port int) {
	cfg := engine.Cfg
	tmpl, err := template.New("dashboard").Parse(webDashboardHTML)
	if err != nil {
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		status, dateKey, _ := GetStatus(cfg.DataDir)
		if IsLoggedToday(cfg.DataDir) {
			status = "in"
			dateKey = LocalDateKey(time.Now())
		}
		statusText, _ := GetStatusDisplay(status, dateKey, true)

		var logs string
		if data, err := os.ReadFile(AttendanceLogFilePath(cfg.DataDir)); err == nil {
			lines := strings.Split(string(data), "\n")
			if len(lines) > 50 {
				lines = lines[len(lines)-50:]
			}
			logs = strings.Join(lines, "\n")
		}

		barH := cfg.BarHeight
		if barH <= 0 {
			barH = 2
		}

		data := dashboardData{
			StatusClass:   status,
			StatusText:    statusText,
			CompanyName:   cfg.CompanyName,
			ClockInMode:   string(cfg.ClockInMode),
			BarHeight:     barH,
			SkipFrom:      cfg.SkipCheckFrom,
			SkipUntil:     cfg.SkipCheckUntil,
			CheckInterval: fmt.Sprintf("%d seconds", int(cfg.CheckInterval.Seconds())),
			Port:          port,
			Logs:          logs,
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = tmpl.Execute(w, data)
	})

	mux.HandleFunc("/api/check", func(w http.ResponseWriter, r *http.Request) {
		engine.TriggerCheck()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "ok",
			"message": "Attendance check triggered",
		})
	})

	mux.HandleFunc("/api/stop", func(w http.ResponseWriter, r *http.Request) {
		Log(cfg.DataDir, "Daemon stopped via Web UI dashboard")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "ok",
			"message": "Stopping attendance daemon",
		})

		go func() {
			time.Sleep(200 * time.Millisecond)
			profileDir := engine.Driver.GetDebugProfileDir(cfg.BaseDir)
			_ = engine.Driver.StopAttendanceProcesses(profileDir, cfg.DebugPort)
			_ = os.Remove(LockFilePath(cfg.DataDir))
			os.Exit(0)
		}()
	})

	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		status, dateKey, _ := GetStatus(cfg.DataDir)
		loggedToday := IsLoggedToday(cfg.DataDir)
		if loggedToday {
			status = "in"
			dateKey = LocalDateKey(time.Now())
		}
		statusText, color := GetStatusDisplay(status, dateKey, true)

		var logs string
		if data, err := os.ReadFile(AttendanceLogFilePath(cfg.DataDir)); err == nil {
			lines := strings.Split(string(data), "\n")
			if len(lines) > 50 {
				lines = lines[len(lines)-50:]
			}
			logs = strings.Join(lines, "\n")
		}

		res := map[string]any{
			"status":       status,
			"status_text":  statusText,
			"color":        color,
			"logged_today": loggedToday,
			"date":         dateKey,
			"company":      cfg.CompanyName,
			"mode":         cfg.ClockInMode,
			"bar_height":   cfg.BarHeight,
			"logs":         logs,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
	})

	server := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", port),
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		_ = server.Close()
	}()

	_ = server.ListenAndServe()
}
