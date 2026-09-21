package api

import (
	"net/http"
	"strconv"
	"time"

	"splitfriends/internal/auth"
	"splitfriends/internal/httpx"
	"splitfriends/internal/realtime"
	"splitfriends/internal/store"
)

const pingInterval = 25 * time.Second

func (s *Server) groupEvents(w http.ResponseWriter, r *http.Request) {
	g := groupFrom(r.Context())
	since, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	evs, err := store.GroupEvents(r.Context(), s.db, g.ID, since, limit)
	if err != nil {
		httpx.Internal(w, err)
		return
	}
	httpx.JSON(w, 200, evs)
}

func (s *Server) activity(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	evs, err := store.Activity(r.Context(), s.db, u.ID, limit)
	if err != nil {
		httpx.Internal(w, err)
		return
	}
	httpx.JSON(w, 200, evs)
}

// stream is the single SSE connection per client. It replays anything after
// Last-Event-ID (header or ?last_event_id=) and then forwards live events.
func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpx.Error(w, 500, "no_streaming", "streaming unsupported")
		return
	}
	last, _ := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64)
	if q := r.URL.Query().Get("last_event_id"); q != "" && last == 0 {
		last, _ = strconv.ParseInt(q, 10, 64)
	}

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache, no-transform")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// Subscribe before replaying so nothing published in between is lost;
	// a duplicate id is harmless for the client, a gap is not.
	client := s.hub.Subscribe(u.ID)
	defer s.hub.Unsubscribe(client)

	if last > 0 {
		missed, err := store.EventsAfter(r.Context(), s.db, u.ID, last, 1000)
		if err == nil {
			for _, ev := range missed {
				_, _ = w.Write(realtime.Frame(ev))
			}
		}
	}
	_, _ = w.Write([]byte(": connected\n\n"))
	flusher.Flush()

	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case frame, ok := <-client.Ch:
			if !ok {
				return
			}
			if _, err := w.Write(frame); err != nil {
				return
			}
			flusher.Flush()
		case <-ticker.C:
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
