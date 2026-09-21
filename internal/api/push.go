package api

import (
	"net/http"

	"splitfriends/internal/auth"
	"splitfriends/internal/httpx"
	"splitfriends/internal/store"
)

func (s *Server) pushVapid(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, 200, map[string]string{"public_key": s.push.Public})
}

type subscriptionJSON struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

func (s *Server) pushSubscribe(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	var in subscriptionJSON
	if !httpx.Decode(w, r, &in) {
		return
	}
	if in.Endpoint == "" || in.Keys.P256dh == "" || in.Keys.Auth == "" {
		httpx.Error(w, 400, "invalid_subscription", "endpoint and keys are required")
		return
	}
	err := store.UpsertPushSub(r.Context(), s.db, store.PushSub{UserID: u.ID, Endpoint: in.Endpoint, P256dh: in.Keys.P256dh, Auth: in.Keys.Auth})
	if err != nil {
		httpx.Internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) pushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Endpoint string `json:"endpoint"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	if err := store.DeletePushSub(r.Context(), s.db, in.Endpoint); err != nil {
		httpx.Internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
