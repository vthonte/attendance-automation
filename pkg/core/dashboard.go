package core

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
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
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #0f141c; color: #e1e7ec; padding: 24px; }
    .container { max-width: 720px; margin: 0 auto; }
    .card { background: #18202c; border: 1px solid #283446; border-radius: 12px; padding: 24px; margin-bottom: 20px; box-shadow: 0 4px 20px rgba(0,0,0,0.3); }
    h1 { font-size: 22px; font-weight: 600; margin-bottom: 16px; display: flex; align-items: center; justify-content: space-between; }
    .badge { display: inline-block; padding: 6px 14px; border-radius: 20px; font-size: 14px; font-weight: 600; }
    .badge.in { background: rgba(144, 238, 144, 0.2); color: #90ee90; border: 1px solid #90ee90; }
    .badge.run { background: rgba(240, 230, 140, 0.2); color: #f0e68c; border: 1px solid #f0e68c; }
    .badge.error { background: rgba(240, 128, 128, 0.2); color: #f08080; border: 1px solid #f08080; }
    .badge.out { background: rgba(211, 211, 211, 0.2); color: #d3d3d3; border: 1px solid #d3d3d3; }
    .info-grid { display: grid; grid-template-columns: repeat(2, 1fr); gap: 16px; margin: 20px 0; }
    .info-item { background: #111722; padding: 12px 16px; border-radius: 8px; border: 1px solid #1f2a3a; }
    .info-label { font-size: 12px; color: #8a99ad; margin-bottom: 4px; text-transform: uppercase; letter-spacing: 0.5px; }
    .info-value { font-size: 15px; font-weight: 500; }
    .logs-box { background: #0b0f16; border: 1px solid #1f2a3a; border-radius: 8px; padding: 14px; font-family: monospace; font-size: 12px; line-height: 1.5; max-height: 240px; overflow-y: auto; color: #a9b7c6; white-space: pre-wrap; }
    .actions { display: flex; gap: 12px; margin-top: 16px; }
    button { background: #2563eb; color: white; border: none; padding: 10px 18px; border-radius: 6px; font-weight: 500; font-size: 14px; cursor: pointer; transition: background 0.2s; }
    button:hover { background: #1d4ed8; }
  </style>
</head>
<body>
  <div class="container">
    <div class="card">
      <h1>
        <span>Attendance Automation</span>
        <span class="badge {{.StatusClass}}">{{.StatusText}}</span>
      </h1>
      <p style="color: #8a99ad; font-size: 14px;">Automated Keka Clock-In Service</p>

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
          <div class="info-label">Quiet Window</div>
          <div class="info-value">{{.SkipFrom}} - {{.SkipUntil}}</div>
        </div>
        <div class="info-item">
          <div class="info-label">Check Interval</div>
          <div class="info-value">{{.CheckInterval}}</div>
        </div>
      </div>

      <div class="actions">
        <button onclick="location.reload()">Refresh Status</button>
      </div>
    </div>

    <div class="card">
      <h2 style="font-size: 16px; margin-bottom: 12px; color: #cbd5e1;">Recent Logs</h2>
      <div class="logs-box">{{.Logs}}</div>
    </div>
  </div>
</body>
</html>`

type dashboardData struct {
	StatusClass   string
	StatusText    string
	CompanyName   string
	ClockInMode   string
	SkipFrom      string
	SkipUntil     string
	CheckInterval string
	Logs          string
}

func StartWebDashboard(ctx context.Context, cfg *Config, port int) {
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

		data := dashboardData{
			StatusClass:   status,
			StatusText:    statusText,
			CompanyName:   cfg.CompanyName,
			ClockInMode:   string(cfg.ClockInMode),
			SkipFrom:      cfg.SkipCheckFrom,
			SkipUntil:     cfg.SkipCheckUntil,
			CheckInterval: fmt.Sprintf("%d seconds", int(cfg.CheckInterval.Seconds())),
			Logs:          logs,
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = tmpl.Execute(w, data)
	})

	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		status, dateKey, _ := GetStatus(cfg.DataDir)
		loggedToday := IsLoggedToday(cfg.DataDir)
		if loggedToday {
			status = "in"
			dateKey = LocalDateKey(time.Now())
		}
		statusText, color := GetStatusDisplay(status, dateKey, true)

		res := map[string]any{
			"status":       status,
			"status_text":  statusText,
			"color":        color,
			"logged_today": loggedToday,
			"date":         dateKey,
			"company":      cfg.CompanyName,
			"mode":         cfg.ClockInMode,
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
