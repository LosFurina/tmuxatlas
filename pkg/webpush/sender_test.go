package webpush

import (
	"errors"
	"net/http"
	"path/filepath"
	"testing"

	wp "github.com/SherClockHolmes/webpush-go"
	"github.com/sirupsen/logrus"

	"github.com/LosFurina/tmuxatlas/pkg/preferences"
	"github.com/LosFurina/tmuxatlas/pkg/toolevents"
)

type fakePreferences struct {
	value *preferences.Preferences
}

func (f fakePreferences) Get() *preferences.Preferences { return f.value }

func TestSenderFiltersCurrentPreferences(t *testing.T) {
	prefs := preferences.Default()
	prefs.Notifications.Statuses = []string{"completed"}
	sender := &Sender{prefs: fakePreferences{value: prefs}}
	if sender.statusEnabled(toolevents.StatusWaiting) {
		t.Fatal("waiting should be filtered")
	}
	if !sender.statusEnabled(toolevents.StatusCompleted) {
		t.Fatal("completed should be enabled")
	}
	prefs.Notifications.Statuses = []string{"error"}
	if !sender.statusEnabled(toolevents.StatusError) {
		t.Fatal("sender did not read updated preferences")
	}
}

func TestBuildPayloadUsesStableRemoteIdentity(t *testing.T) {
	payload, ok := buildPayload(&toolevents.Event{
		Tool: toolevents.ToolCodex, Status: toolevents.StatusCompleted,
		Host: "host-a", HostName: "duplicate", Session: "work",
		Window: 2, Pane: "%3",
	})
	if !ok {
		t.Fatal("payload rejected")
	}
	if payload.HostID != "host-a" || payload.HostName != "duplicate" ||
		payload.Session != "work" || payload.Pane != "%3" {
		t.Fatalf("payload = %+v", payload)
	}
	if _, ok := buildPayload(&toolevents.Event{
		Tool: toolevents.ToolCodex, Status: toolevents.StatusWaiting, Session: "work",
	}); ok {
		t.Fatal("session payload without stable host identity should fall back, not send an ambiguous target")
	}
}

func TestSenderPersistsExpiredEndpointRemoval(t *testing.T) {
	store, err := NewStoreAt(filepath.Join(t.TempDir(), subscriptionsFileName))
	if err != nil {
		t.Fatal(err)
	}
	subscription := &wp.Subscription{Endpoint: "https://push.example/expired"}
	if err := store.Add(subscription); err != nil {
		t.Fatal(err)
	}
	sender := &Sender{
		keys: &VAPIDKeys{}, store: store,
		logger: logrus.New().WithField("test", true),
		send: func([]byte, *wp.Subscription, *wp.Options) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusGone}, errors.New("gone")
		},
	}
	sender.sendAll(&toolevents.Event{
		Tool: toolevents.ToolCodex, Status: toolevents.StatusWaiting,
		Host: "host-a", Session: "work",
	})
	if store.Count() != 0 {
		t.Fatal("expired endpoint was not removed")
	}
	reloaded, err := NewStoreAt(filepath.Join(filepath.Dir(store.path), subscriptionsFileName))
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Count() != 0 {
		t.Fatal("expired endpoint removal was not durable")
	}
}
