// Package push sends Web Push notifications using VAPID. Keys are generated
// once and persisted in the settings table so subscriptions survive restarts.
package push

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"

	"splitfriends/internal/db"
	"splitfriends/internal/store"
)

type Service struct {
	db      *db.DB
	Public  string
	private string
	contact string
	client  *http.Client
}

// Payload is what the service worker receives.
type Payload struct {
	Title   string `json:"title"`
	Body    string `json:"body"`
	URL     string `json:"url"`
	Tag     string `json:"tag"`
	GroupID string `json:"group_id"`
}

func New(ctx context.Context, d *db.DB, contact string) (*Service, error) {
	pub, ok1, err := d.Setting(ctx, "vapid_public")
	if err != nil {
		return nil, err
	}
	priv, ok2, err := d.Setting(ctx, "vapid_private")
	if err != nil {
		return nil, err
	}
	if !ok1 || !ok2 {
		priv, pub, err = webpush.GenerateVAPIDKeys()
		if err != nil {
			return nil, err
		}
		if err := d.SetSetting(ctx, "vapid_public", pub); err != nil {
			return nil, err
		}
		if err := d.SetSetting(ctx, "vapid_private", priv); err != nil {
			return nil, err
		}
		slog.Info("generated new VAPID key pair")
	}
	return &Service{db: d, Public: pub, private: priv, contact: contact,
		client: &http.Client{Timeout: 10 * time.Second}}, nil
}

// Notify sends payload to every subscription of the given users. It runs
// synchronously; callers wrap it in a goroutine.
func (s *Service) Notify(ctx context.Context, userIDs []string, p Payload) {
	if len(userIDs) == 0 {
		return
	}
	subs, err := store.PushSubsForUsers(ctx, s.db, userIDs)
	if err != nil {
		slog.Error("push: load subscriptions", "err", err)
		return
	}
	body, _ := json.Marshal(p)
	for _, sub := range subs {
		resp, err := webpush.SendNotificationWithContext(ctx, body, &webpush.Subscription{
			Endpoint: sub.Endpoint,
			Keys:     webpush.Keys{P256dh: sub.P256dh, Auth: sub.Auth},
		}, &webpush.Options{
			HTTPClient:      s.client,
			Subscriber:      s.contact,
			VAPIDPublicKey:  s.Public,
			VAPIDPrivateKey: s.private,
			TTL:             60 * 60 * 24,
			Topic:           p.Tag,
			Urgency:         webpush.UrgencyNormal,
		})
		if err != nil {
			slog.Warn("push: send failed", "endpoint", sub.Endpoint[:min(40, len(sub.Endpoint))], "err", err)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
			// Subscription expired or was revoked; forget it.
			_ = store.DeletePushSub(ctx, s.db, sub.Endpoint)
		} else if resp.StatusCode >= 400 {
			slog.Warn("push: rejected", "status", resp.StatusCode)
		}
	}
}
