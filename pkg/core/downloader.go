package core

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type cftMetadata struct {
	Channels struct {
		Stable struct {
			Version   string `json:"version"`
			Downloads struct {
				ChromeHeadlessShell []struct {
					Platform string `json:"platform"`
					URL      string `json:"url"`
				} `json:"chrome-headless-shell"`
			} `json:"downloads"`
		} `json:"Stable"`
	} `json:"channels"`
}

func getCFTPlatform() string {
	switch runtime.GOOS {
	case "windows":
		return "win64"
	case "darwin":
		if runtime.GOARCH == "arm64" {
			return "mac-arm64"
		}
		return "mac-x64"
	case "linux":
		return "linux64"
	default:
		return "linux64"
	}
}

func DownloadPortableBrowser(dataDir string) error {
	browserDir := filepath.Join(dataDir, "browser")
	if err := os.MkdirAll(browserDir, 0755); err != nil {
		return fmt.Errorf("failed to create browser dir: %w", err)
	}

	platformKey := getCFTPlatform()
	Log(dataDir, fmt.Sprintf("Checking latest Chrome Headless Shell for %s (%s/%s)...", platformKey, runtime.GOOS, runtime.GOARCH))

	metaURL := "https://googlechromelabs.github.io/chrome-for-testing/last-known-good-versions-with-downloads.json"
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metaURL, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch browser metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status fetching browser metadata: %d", resp.StatusCode)
	}

	var meta cftMetadata
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return fmt.Errorf("failed to parse browser metadata: %w", err)
	}

	var downloadURL string
	for _, item := range meta.Channels.Stable.Downloads.ChromeHeadlessShell {
		if item.Platform == platformKey {
			downloadURL = item.URL
			break
		}
	}

	if downloadURL == "" {
		return fmt.Errorf("no chrome-headless-shell download found for platform %s", platformKey)
	}

	Log(dataDir, fmt.Sprintf("Downloading Chrome Headless Shell v%s from %s...", meta.Channels.Stable.Version, downloadURL))

	zipFile := filepath.Join(browserDir, "browser.zip")
	defer os.Remove(zipFile)

	dlReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}

	dlResp, err := http.DefaultClient.Do(dlReq)
	if err != nil {
		return fmt.Errorf("failed to download browser: %w", err)
	}
	defer dlResp.Body.Close()

	out, err := os.Create(zipFile)
	if err != nil {
		return fmt.Errorf("failed to create zip file: %w", err)
	}

	_, err = io.Copy(out, dlResp.Body)
	out.Close()
	if err != nil {
		return fmt.Errorf("failed to save browser zip: %w", err)
	}

	Log(dataDir, "Extracting browser archive...")

	zr, err := zip.OpenReader(zipFile)
	if err != nil {
		return fmt.Errorf("failed to open zip file: %w", err)
	}
	defer zr.Close()

	for _, f := range zr.File {
		targetPath := filepath.Join(browserDir, f.Name)
		if !strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(browserDir)+string(os.PathSeparator)) {
			continue
		}

		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(targetPath, 0755)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}

		outFile, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
		if err != nil {
			return err
		}

		if runtime.GOOS != "windows" {
			_ = os.Chmod(targetPath, 0755)
		}
	}

	Log(dataDir, "Portable Chrome Headless Shell installed successfully.")
	return nil
}
