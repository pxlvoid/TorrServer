package torrstor

// homelab: tests of the ntfy notifications (homelab_ntfy.go).

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"server/settings"
)

type ntfyStub struct {
	mu   sync.Mutex
	got  []map[string]any
	auth []string
}

func newNtfyStub(t *testing.T, status int) (*ntfyStub, *httptest.Server) {
	stub := &ntfyStub{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		stub.mu.Lock()
		stub.got = append(stub.got, body)
		stub.auth = append(stub.auth, r.Header.Get("Authorization"))
		stub.mu.Unlock()
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
	}))
	t.Cleanup(srv.Close)
	return stub, srv
}

func (s *ntfyStub) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.got)
}

// JSON to the server root with the topic in the body: Cyrillic titles pass, the token goes as Bearer.
func TestHomelabNtfySend(t *testing.T) {
	stub, srv := newNtfyStub(t, http.StatusOK)
	cfg := settings.HomelabNtfy{URL: srv.URL, Topic: "torrserver", Token: "tk_secret"}
	err := hlNtfySend(cfg, HomelabMessage{Title: "Скачано на диск: Пацаны", Message: "весь торрент", Priority: 4, Tags: []string{"arrow_down"}})
	if err != nil {
		t.Fatal(err)
	}
	b := stub.got[0]
	if b["topic"] != "torrserver" || b["title"] != "Скачано на диск: Пацаны" || b["priority"] != float64(4) || stub.auth[0] != "Bearer tk_secret" {
		t.Fatalf("body %+v auth %q", b, stub.auth[0])
	}

	_, bad := newNtfyStub(t, http.StatusForbidden)
	if err := HomelabNotifyTest(settings.HomelabNtfy{URL: bad.URL, Topic: "torrserver"}); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("a refused notification must be an error with the status: %v", err)
	}
}

// Events off and repeats within the interval are not sent.
func TestHomelabNotifyEvents(t *testing.T) {
	stub, srv := newNtfyStub(t, http.StatusOK)
	prev := settings.GetHomelabNtfy()
	t.Cleanup(func() { settings.SetHomelabNtfyForTest(prev) })
	settings.SetHomelabNtfyForTest(settings.HomelabNtfy{URL: srv.URL, Topic: "t",
		Events: map[string]bool{HomelabEvDisk: true, HomelabEvStarted: false}})

	HomelabNotify(HomelabMessage{Event: HomelabEvStarted, Title: "off"})
	HomelabNotifyEvery("test-disk", time.Hour, HomelabMessage{Event: HomelabEvDisk, Title: "1"})
	HomelabNotifyEvery("test-disk", time.Hour, HomelabMessage{Event: HomelabEvDisk, Title: "2"})
	deadline := time.Now().Add(3 * time.Second)
	for stub.count() < 1 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond) // a wrongly sent second one would arrive by now
	if n := stub.count(); n != 1 || stub.got[0]["title"] != "1" {
		t.Fatalf("sent %d: %+v", n, stub.got)
	}
}
