package api

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"splitfriends/internal/auth"
	"splitfriends/internal/httpx"
	"splitfriends/internal/ledger"
	"splitfriends/internal/push"
	"splitfriends/internal/store"
)

const inviteTTL = 7 * 24 * time.Hour

type groupDetail struct {
	Group      ledger.Group     `json:"group"`
	Expenses   []ledger.Expense `json:"expenses"`
	Payments   []ledger.Payment `json:"payments"`
	Balances   []ledger.Balance `json:"balances"`
	Pairwise   []ledger.Debt    `json:"pairwise"`
	Simplified []ledger.Debt    `json:"simplified"`
}

func (s *Server) listGroups(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	groups, err := store.GroupsForUser(r.Context(), s.db, u.ID)
	if err != nil {
		httpx.Internal(w, err)
		return
	}
	httpx.JSON(w, 200, groups)
}

func validCurrency(c string) bool {
	if len(c) != 3 {
		return false
	}
	for _, ch := range c {
		if ch < 'A' || ch > 'Z' {
			return false
		}
	}
	return true
}

func (s *Server) createGroup(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	var in struct {
		Name     string `json:"name"`
		Currency string `json:"currency"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	if in.Name == "" || len(in.Name) > 80 {
		httpx.Error(w, 400, "invalid_name", "group name must be 1-80 characters")
		return
	}
	if !validCurrency(in.Currency) {
		httpx.Error(w, 400, "invalid_currency", "currency must be a 3-letter ISO code")
		return
	}
	var g ledger.Group
	err := s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		var err error
		if g, err = store.CreateGroup(r.Context(), tx, in.Name, in.Currency, u.ID); err != nil {
			return err
		}
		m, err := store.AddMember(r.Context(), tx, g.ID, u.ID)
		if err != nil {
			return err
		}
		g.Members = []ledger.Member{m}
		ev, err := store.AppendEvent(r.Context(), tx, g.ID, ledger.EvGroupCreated, u, map[string]any{"group": g})
		if err != nil {
			return err
		}
		g.LastSeq = ev.Seq
		return nil
	})
	if err != nil {
		httpx.Internal(w, err)
		return
	}
	httpx.JSON(w, 201, g)
}

func (s *Server) loadDetail(r *http.Request, g ledger.Group, me string) (groupDetail, error) {
	exps, err := store.Expenses(r.Context(), s.db, g.ID)
	if err != nil {
		return groupDetail{}, err
	}
	pays, err := store.Payments(r.Context(), s.db, g.ID)
	if err != nil {
		return groupDetail{}, err
	}
	ids := make([]string, len(g.Members))
	for i, m := range g.Members {
		ids[i] = m.UserID
	}
	bal := ledger.Balances(ids, exps, pays)
	for _, b := range bal {
		if b.UserID == me {
			g.MyBalance = b.Net
		}
	}
	d := groupDetail{Group: g, Expenses: exps, Payments: pays, Balances: bal,
		Pairwise: ledger.Pairwise(exps, pays), Simplified: ledger.Simplify(bal)}
	if d.Pairwise == nil {
		d.Pairwise = []ledger.Debt{}
	}
	if d.Simplified == nil {
		d.Simplified = []ledger.Debt{}
	}
	return d, nil
}

func (s *Server) getGroup(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	d, err := s.loadDetail(r, groupFrom(r.Context()), u.ID)
	if err != nil {
		httpx.Internal(w, err)
		return
	}
	httpx.JSON(w, 200, d)
}

func (s *Server) updateGroup(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	g := groupFrom(r.Context())
	var in struct {
		Name     *string `json:"name"`
		Currency *string `json:"currency"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		if n == "" || len(n) > 80 {
			httpx.Error(w, 400, "invalid_name", "group name must be 1-80 characters")
			return
		}
		g.Name = n
	}
	if in.Currency != nil {
		c := strings.ToUpper(strings.TrimSpace(*in.Currency))
		if !validCurrency(c) {
			httpx.Error(w, 400, "invalid_currency", "currency must be a 3-letter ISO code")
			return
		}
		g.Currency = c
	}
	ev, err := s.commit(r.Context(), g.ID, u, func(tx *sql.Tx) (ledger.Event, push.Payload, error) {
		if err := store.UpdateGroup(r.Context(), tx, g.ID, g.Name, g.Currency); err != nil {
			return ledger.Event{}, push.Payload{}, err
		}
		ev, err := store.AppendEvent(r.Context(), tx, g.ID, ledger.EvGroupUpdated, u, map[string]any{"group": g})
		return ev, push.Payload{}, err
	})
	if err != nil {
		handleErr(w, err)
		return
	}
	g.LastSeq = ev.Seq
	httpx.JSON(w, 200, g)
}

func (s *Server) leaveGroup(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	g := groupFrom(r.Context())
	d, err := s.loadDetail(r, g, u.ID)
	if err != nil {
		httpx.Internal(w, err)
		return
	}
	if d.Group.MyBalance != 0 {
		httpx.Error(w, 400, "nonzero_balance", "settle up before leaving the group")
		return
	}
	var me ledger.Member
	for _, m := range g.Members {
		if m.UserID == u.ID {
			me = m
		}
	}
	_, err = s.commit(r.Context(), g.ID, u, func(tx *sql.Tx) (ledger.Event, push.Payload, error) {
		if err := store.RemoveMember(r.Context(), tx, g.ID, u.ID); err != nil {
			return ledger.Event{}, push.Payload{}, err
		}
		ev, err := store.AppendEvent(r.Context(), tx, g.ID, ledger.EvMemberLeft, u, map[string]any{"member": me})
		return ev, push.Payload{}, err
	})
	if err != nil {
		handleErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) createInvite(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	g := groupFrom(r.Context())
	inv, err := store.CreateInvite(r.Context(), s.db, g.ID, u.ID, inviteTTL)
	if err != nil {
		httpx.Internal(w, err)
		return
	}
	httpx.JSON(w, 201, map[string]string{
		"token":      inv.Token,
		"url":        s.cfg.Origin + "/join/" + inv.Token,
		"expires_at": inv.ExpiresAt,
	})
}

func (s *Server) getInvite(w http.ResponseWriter, r *http.Request) {
	inv, err := store.GetInvite(r.Context(), s.db, chi.URLParam(r, "token"))
	if err != nil {
		handleErr(w, err)
		return
	}
	g, err := store.GetGroup(r.Context(), s.db, inv.GroupID)
	if err != nil {
		handleErr(w, err)
		return
	}
	inviter, err := store.GetUser(r.Context(), s.db, inv.CreatedBy)
	if err != nil {
		handleErr(w, err)
		return
	}
	already := false
	if u, ok := auth.UserFrom(r.Context()); ok {
		for _, m := range g.Members {
			if m.UserID == u.ID {
				already = true
			}
		}
	}
	httpx.JSON(w, 200, map[string]any{
		"group_id": g.ID, "group_name": g.Name, "currency": g.Currency,
		"inviter_name": inviter.Name, "member_count": len(g.Members), "already_member": already,
	})
}

func (s *Server) acceptInvite(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	inv, err := store.GetInvite(r.Context(), s.db, chi.URLParam(r, "token"))
	if err != nil {
		handleErr(w, err)
		return
	}
	already, err := store.IsMember(r.Context(), s.db, inv.GroupID, u.ID)
	if err != nil {
		httpx.Internal(w, err)
		return
	}
	if !already {
		_, err = s.commit(r.Context(), inv.GroupID, u, func(tx *sql.Tx) (ledger.Event, push.Payload, error) {
			m, err := store.AddMember(r.Context(), tx, inv.GroupID, u.ID)
			if err != nil {
				return ledger.Event{}, push.Payload{}, err
			}
			g, err := store.GetGroup(r.Context(), tx, inv.GroupID)
			if err != nil {
				return ledger.Event{}, push.Payload{}, err
			}
			ev, err := store.AppendEvent(r.Context(), tx, inv.GroupID, ledger.EvMemberJoined, u, map[string]any{"member": m})
			pl := push.Payload{Title: g.Name, Body: u.Name + " joined the group", Tag: "member-" + inv.GroupID}
			return ev, pl, err
		})
		if err != nil {
			handleErr(w, err)
			return
		}
	}
	groups, err := store.GroupsForUser(r.Context(), s.db, u.ID)
	if err != nil {
		httpx.Internal(w, err)
		return
	}
	for _, g := range groups {
		if g.ID == inv.GroupID {
			httpx.JSON(w, 200, g)
			return
		}
	}
	httpx.Error(w, 500, "internal", "group vanished")
}
