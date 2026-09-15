package torrstor

// homelab: ntfy notifications (settings.HomelabNtfy): downloads to disk, the cache not fitting, the server started.
// Here rather than in torr: the janitor (this package) notifies too, and torr imports torrstor.
// Sent as JSON to the server root — in HTTP headers (Title) Cyrillic does not pass; same as homelab-upstream.yml.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"server/log"
	"server/settings"
)

// Events of the notifications (settings.HomelabNtfyEvents).
const (
	HomelabEvDownloadDone  = "download_done"
	HomelabEvDownloadError = "download_error"
	HomelabEvDisk          = "disk"
	HomelabEvStarted       = "started"
)

// HomelabMessage — a notification.
type HomelabMessage struct {
	Event    string
	Title    string
	Message  string
	Priority int      // 1..5, 0 — the ntfy default (3)
	Tags     []string // ntfy tags / emoji shortcodes
}

var (
	hlNtfyClient = &http.Client{Timeout: 10 * time.Second}
	hlNtfyMu     sync.Mutex
	hlNtfyLast   = map[string]time.Time{} // key → last sent, for HomelabNotifyEvery
)

// HomelabNotify sends a notification in the background if ntfy is set up and the event is on.
func HomelabNotify(m HomelabMessage) {
	cfg := settings.GetHomelabNtfy()
	if cfg.URL == "" || !cfg.Events[m.Event] {
		return
	}
	go func() {
		if err := hlNtfySend(cfg, m); err != nil {
			log.TLogln("homelab: ntfy:", err)
		}
	}()
}

// HomelabNotifyEvery — HomelabNotify, but for the same key not more often than every.
func HomelabNotifyEvery(key string, every time.Duration, m HomelabMessage) {
	hlNtfyMu.Lock()
	if last, ok := hlNtfyLast[key]; ok && time.Since(last) < every {
		hlNtfyMu.Unlock()
		return
	}
	hlNtfyLast[key] = time.Now()
	hlNtfyMu.Unlock()
	HomelabNotify(m)
}

// HomelabNotifyTest sends a test notification with the given settings, whatever the events; the error is for the UI.
func HomelabNotifyTest(cfg settings.HomelabNtfy) error {
	if cfg.URL == "" {
		return errors.New("ntfy address is not set")
	}
	return hlNtfySend(cfg, HomelabMessage{
		Title:   "TorrServer: проверка уведомлений",
		Message: "Если вы это видите — уведомления из TorrServer доходят.",
		Tags:    []string{"white_check_mark"},
	})
}

func hlNtfySend(cfg settings.HomelabNtfy, m HomelabMessage) error {
	body := map[string]any{"topic": cfg.Topic, "title": m.Title, "message": m.Message}
	if m.Priority > 0 {
		body["priority"] = m.Priority
	}
	if len(m.Tags) > 0 {
		body["tags"] = m.Tags
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(cfg.URL, "/"), bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	resp, err := hlNtfyClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("ntfy answered %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	return nil
}

// hlGB — bytes for a message.
func hlGB(n int64) string {
	return fmt.Sprintf("%.1f ГБ", float64(n)/(1<<30))
}
