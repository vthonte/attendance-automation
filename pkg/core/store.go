package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	storeMu sync.Mutex
	logMu   sync.Mutex
)

func LocalDateKey(t time.Time) string {
	return t.Format("2006-01-02")
}

func StoreFilePath(dataDir string) string {
	return filepath.Join(dataDir, "attendance_store.json")
}

func StatusFilePath(dataDir string) string {
	return filepath.Join(dataDir, "toast_status.txt")
}

func AttendanceLogFilePath(dataDir string) string {
	return filepath.Join(dataDir, "attendance_log.txt")
}

func ToastLogFilePath(dataDir string) string {
	return filepath.Join(dataDir, "toast_log.txt")
}

func LockFilePath(dataDir string) string {
	return filepath.Join(dataDir, "attendance_lock.txt")
}

func LoadStore(dataDir string) map[string]bool {
	storeMu.Lock()
	defer storeMu.Unlock()

	storeFile := StoreFilePath(dataDir)
	result := make(map[string]bool)
	data, err := os.ReadFile(storeFile)
	if err != nil {
		return result
	}
	_ = json.Unmarshal(data, &result)
	return result
}

func SaveStore(dataDir string, st map[string]bool) error {
	storeMu.Lock()
	defer storeMu.Unlock()

	storeFile := StoreFilePath(dataDir)
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(storeFile, data, 0644)
}

func IsLoggedToday(dataDir string) bool {
	st := LoadStore(dataDir)
	return st[LocalDateKey(time.Now())]
}

func MarkLoggedToday(dataDir string) error {
	st := LoadStore(dataDir)
	st[LocalDateKey(time.Now())] = true
	return SaveStore(dataDir, st)
}

func SetStatus(dataDir, status string) error {
	storeMu.Lock()
	defer storeMu.Unlock()

	statusFile := StatusFilePath(dataDir)
	tmpFile := statusFile + ".tmp"

	statusText := status
	if status == "in" {
		statusText = fmt.Sprintf("%s %s", status, LocalDateKey(time.Now()))
	}

	if err := os.WriteFile(tmpFile, []byte(statusText), 0644); err != nil {
		return err
	}
	return os.Rename(tmpFile, statusFile)
}

func GetStatus(dataDir string) (status string, dateKey string, err error) {
	statusFile := StatusFilePath(dataDir)
	data, err := os.ReadFile(statusFile)
	if err != nil {
		return "out", "", err
	}
	parts := strings.Fields(strings.TrimSpace(string(data)))
	if len(parts) == 0 {
		return "out", "", nil
	}
	status = parts[0]
	if len(parts) > 1 {
		dateKey = parts[1]
	}
	return status, dateKey, nil
}

func Log(dataDir, message string) {
	logMu.Lock()
	defer logMu.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	line := fmt.Sprintf("[%s] %s\n", timestamp, message)

	fmt.Print(line)

	if dataDir != "" {
		logFile := AttendanceLogFilePath(dataDir)
		if f, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644); err == nil {
			defer f.Close()
			_, _ = f.WriteString(line)
		}
	}
}

func LogToast(dataDir, message string) {
	logMu.Lock()
	defer logMu.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	line := fmt.Sprintf("[%s] %s\n", timestamp, message)

	if dataDir != "" {
		logFile := ToastLogFilePath(dataDir)
		if f, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644); err == nil {
			defer f.Close()
			_, _ = f.WriteString(line)
		}
	}
}

func AcquireLock(dataDir string, driver PlatformDriver) (func(), error) {
	lockFile := LockFilePath(dataDir)
	pid := os.Getpid()

	if data, err := os.ReadFile(lockFile); err == nil {
		oldPidStr := strings.TrimSpace(string(data))
		if oldPid, err := strconv.Atoi(oldPidStr); err == nil && oldPid != pid {
			if driver.IsProcessRunning(oldPid) {
				Log(dataDir, fmt.Sprintf("Existing attendance process running (PID: %d). Stopping previous instance to restart...", oldPid))
				_ = driver.KillProcess(oldPid)
				for i := 0; i < 15; i++ {
					if !driver.IsProcessRunning(oldPid) {
						break
					}
					time.Sleep(100 * time.Millisecond)
				}
			} else {
				Log(dataDir, fmt.Sprintf("Removing stale attendance lock for stopped process: %d", oldPid))
			}
		}
	}

	if err := os.WriteFile(lockFile, []byte(strconv.Itoa(pid)), 0644); err != nil {
		return nil, fmt.Errorf("failed to write lock file: %w", err)
	}

	release := func() {
		if data, err := os.ReadFile(lockFile); err == nil {
			if strings.TrimSpace(string(data)) == strconv.Itoa(pid) {
				_ = os.Remove(lockFile)
			}
		}
	}

	return release, nil
}
