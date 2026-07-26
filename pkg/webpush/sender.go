package webpush

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	wp "github.com/SherClockHolmes/webpush-go"
	"github.com/sirupsen/logrus"

	"github.com/LosFurina/tmuxatlas/pkg/preferences"
	"github.com/LosFurina/tmuxatlas/pkg/toolevents"
)

// PushPayload is the JSON sent to the service worker
type PushPayload struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	HostID   string `json:"host_id"`
	HostName string `json:"host_name,omitempty"`
	Session  string `json:"session"`
	Window   int    `json:"window,omitempty"`
	Pane     string `json:"pane,omitempty"`
	Tool     string `json:"tool"`
	Status   string `json:"status"`
}

type PreferenceSource interface {
	Get() *preferences.Preferences
}

// Sender listens for tool events and sends push notifications
type Sender struct {
	keys    *VAPIDKeys
	store   *Store
	tracker *toolevents.Tracker
	prefs   PreferenceSource
	logger  *logrus.Entry
	send    func([]byte, *wp.Subscription, *wp.Options) (*http.Response, error)
}

// NewSender creates a push notification sender
func NewSender(
	keys *VAPIDKeys,
	store *Store,
	tracker *toolevents.Tracker,
	prefs PreferenceSource,
) *Sender {
	return &Sender{
		keys:    keys,
		store:   store,
		tracker: tracker,
		prefs:   prefs,
		logger:  logrus.WithField("component", "webpush"),
		send:    wp.SendNotification,
	}
}

// Run subscribes to tool events and sends push notifications for waiting/error events
func (s *Sender) Run(ctx context.Context) {
	ch := s.tracker.Subscribe()
	defer s.tracker.Unsubscribe(ch)

	s.logger.Info("push notification sender started")

	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			if !s.statusEnabled(evt.Status) {
				continue
			}
			s.sendAll(evt)
		}
	}
}

func (s *Sender) sendAll(evt *toolevents.Event) {
	subs := s.store.All()
	if len(subs) == 0 {
		return
	}

	payload, ok := buildPayload(evt)
	if !ok {
		return
	}

	data, err := json.Marshal(payload)
	if err != nil {
		s.logger.WithError(err).Error("failed to marshal push payload")
		return
	}

	for _, sub := range subs {
		resp, err := s.send(data, sub, &wp.Options{
			Subscriber:      "mailto:tmuxatlas@localhost",
			VAPIDPublicKey:  s.keys.PublicKey,
			VAPIDPrivateKey: s.keys.PrivateKey,
			TTL:             30,
		})
		if err != nil {
			s.logger.WithError(err).WithField("endpoint", sub.Endpoint).Debug("push send failed")
			// Remove invalid subscriptions (410 Gone or 404)
			if resp != nil && (resp.StatusCode == 410 || resp.StatusCode == 404) {
				if removeErr := s.store.Remove(sub.Endpoint); removeErr != nil {
					s.logger.WithError(removeErr).Warn("failed to persist expired subscription removal")
				}
				s.logger.WithField("endpoint", sub.Endpoint).Info("removed expired subscription")
			}
			continue
		}
		resp.Body.Close()
	}
}

func (s *Sender) statusEnabled(status toolevents.Status) bool {
	if status != toolevents.StatusWaiting &&
		status != toolevents.StatusError &&
		status != toolevents.StatusCompleted {
		return false
	}
	if s.prefs == nil {
		return status == toolevents.StatusWaiting || status == toolevents.StatusError
	}
	for _, enabled := range s.prefs.Get().Notifications.Statuses {
		if enabled == string(status) {
			return true
		}
	}
	return false
}

func buildPayload(evt *toolevents.Event) (PushPayload, bool) {
	if evt == nil || evt.Host == "" || evt.Session == "" {
		return PushPayload{}, false
	}
	var title string
	switch evt.Status {
	case toolevents.StatusWaiting:
		title = fmt.Sprintf("%s needs input", evt.Tool)
	case toolevents.StatusError:
		title = fmt.Sprintf("%s error", evt.Tool)
	case toolevents.StatusCompleted:
		title = fmt.Sprintf("%s completed", evt.Tool)
	default:
		return PushPayload{}, false
	}

	body := fmt.Sprintf("%s in session \"%s\"", evt.Status, evt.Session)
	if evt.Message != "" {
		body += ": " + evt.Message
	}

	return PushPayload{
		Title: title, Body: body, HostID: evt.Host, HostName: evt.HostName,
		Session: evt.Session, Window: evt.Window, Pane: evt.Pane,
		Tool: string(evt.Tool), Status: string(evt.Status),
	}, true
}
