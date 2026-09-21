// Package api wires HTTP routes to the store, hub and push service.
package api

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"splitfriends/internal/auth"
	"splitfriends/internal/config"
	"splitfriends/internal/db"
	"splitfriends/internal/httpx"
	"splitfriends/internal/ledger"
	"splitfriends/internal/push"
	"splitfriends/internal/realtime"
	"splitfriends/internal/store"
)

type Server struct {
	cfg  config.Config
	db   *db.DB
	auth *auth.Service
	hub  *realtime.Hub
	push *push.Service
}

func New(cfg config.Config, d *db.DB, a *auth.Service, h *realtime.Hub, p *push.Service) *Server {
	return &Server{cfg: cfg, db: d, auth: a, hub: h, push: p}
}

// Routes mounts everything under /api.
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(s.auth.Middleware)

	r.Get("/me", s.auth.Me)
	r.Route("/auth", func(r chi.Router) {
		r.Post("/register/begin", s.auth.RegisterBegin)
		r.Post("/register/finish", s.auth.RegisterFinish)
		r.Post("/login/begin", s.auth.LoginBegin)
		r.Post("/login/finish", s.auth.LoginFinish)
		r.Post("/logout", s.auth.Logout)
		r.Group(func(r chi.Router) {
			r.Use(auth.Require)
			r.Get("/passkeys", s.auth.ListPasskeys)
			r.Post("/passkeys/begin", s.auth.AddPasskeyBegin)
			r.Post("/passkeys/finish", s.auth.AddPasskeyFinish)
			r.Delete("/passkeys/{id}", func(w http.ResponseWriter, r *http.Request) {
				s.auth.DeletePasskey(w, r, chi.URLParam(r, "id"))
			})
		})
	})
	r.Get("/invites/{token}", s.getInvite)

	r.Group(func(r chi.Router) {
		r.Use(auth.Require)
		r.Patch("/me", s.auth.UpdateMe)
		r.Get("/stream", s.stream)
		r.Get("/activity", s.activity)
		r.Get("/push/vapid", s.pushVapid)
		r.Post("/push/subscribe", s.pushSubscribe)
		r.Delete("/push/subscribe", s.pushUnsubscribe)
		r.Post("/invites/{token}/accept", s.acceptInvite)

		r.Get("/groups", s.listGroups)
		r.Post("/groups", s.createGroup)
		r.Route("/groups/{id}", func(r chi.Router) {
			r.Use(s.requireMember)
			r.Get("/", s.getGroup)
			r.Patch("/", s.updateGroup)
			r.Post("/leave", s.leaveGroup)
			r.Post("/invites", s.createInvite)
			r.Get("/events", s.groupEvents)
			r.Post("/expenses", s.createExpense)
			r.Patch("/expenses/{eid}", s.updateExpense)
			r.Delete("/expenses/{eid}", s.deleteExpense)
			r.Post("/payments", s.createPayment)
			r.Delete("/payments/{pid}", s.deletePayment)
		})
	})
	return r
}

type groupKey struct{}

// requireMember loads the group and checks membership once per request.
func (s *Server) requireMember(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFrom(r.Context())
		g, err := store.GetGroup(r.Context(), s.db, chi.URLParam(r, "id"))
		if err != nil {
			if err == store.ErrNotFound {
				httpx.Error(w, 404, "not_found", "group not found")
				return
			}
			httpx.Internal(w, err)
			return
		}
		member := false
		for _, m := range g.Members {
			if m.UserID == u.ID {
				member = true
			}
		}
		if !member {
			httpx.Error(w, 404, "not_found", "group not found")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), groupKey{}, g)))
	})
}

func groupFrom(ctx context.Context) ledger.Group { return ctx.Value(groupKey{}).(ledger.Group) }

// commit runs fn in a transaction; fn returns the event to publish. After the
// commit the event is fanned out over SSE and, for disconnected members, push.
func (s *Server) commit(ctx context.Context, groupID string, actor ledger.User, fn func(tx *sql.Tx) (ledger.Event, push.Payload, error)) (ledger.Event, error) {
	var ev ledger.Event
	var pl push.Payload
	var members []string
	err := s.db.Tx(ctx, func(tx *sql.Tx) error {
		var err error
		ev, pl, err = fn(tx)
		if err != nil {
			return err
		}
		members, err = store.MemberIDs(ctx, tx, groupID)
		return err
	})
	if err != nil {
		return ev, err
	}
	s.hub.Publish(members, ev)
	if pl.Title != "" {
		var offline []string
		for _, m := range members {
			if m != actor.ID && !s.hub.Connected(m) {
				offline = append(offline, m)
			}
		}
		pl.GroupID = groupID
		if pl.URL == "" {
			pl.URL = "/groups/" + groupID
		}
		go s.push.Notify(context.Background(), offline, pl)
	}
	return ev, nil
}

// money formats minor units for notification text.
func money(currency string, minor int64) string {
	dec := 2
	switch strings.ToUpper(currency) {
	case "JPY", "KRW", "VND", "CLP", "ISK", "HUF":
		dec = 0
	case "BHD", "KWD", "OMR", "JOD", "IQD", "LYD", "TND":
		dec = 3
	}
	neg := minor < 0
	if neg {
		minor = -minor
	}
	var s string
	switch dec {
	case 0:
		s = fmt.Sprintf("%d", minor)
	default:
		p := int64(1)
		for i := 0; i < dec; i++ {
			p *= 10
		}
		s = fmt.Sprintf("%d.%0*d", minor/p, dec, minor%p)
	}
	if neg {
		s = "-" + s
	}
	return strings.ToUpper(currency) + " " + s
}

func handleErr(w http.ResponseWriter, err error) {
	switch err {
	case nil:
		return
	case store.ErrNotFound:
		httpx.Error(w, 404, "not_found", "not found")
	case ledger.ErrInvalidAmount:
		httpx.Error(w, 400, "invalid_amount", "amount must be a positive whole number of minor units")
	case ledger.ErrPayersMismatch:
		httpx.Error(w, 400, "payers_mismatch", "payer amounts must add up to the total")
	case ledger.ErrSharesMismatch:
		httpx.Error(w, 400, "shares_mismatch", "shares do not add up correctly")
	case ledger.ErrNoParticipants:
		httpx.Error(w, 400, "no_participants", "pick at least one person to split with")
	case errNotMember:
		httpx.Error(w, 400, "not_member", "everyone on the expense must be a group member")
	case errInvalidDate:
		httpx.Error(w, 400, "invalid_date", "date must be YYYY-MM-DD")
	default:
		httpx.Internal(w, err)
	}
}
