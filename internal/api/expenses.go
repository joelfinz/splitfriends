package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"splitfriends/internal/auth"
	"splitfriends/internal/db"
	"splitfriends/internal/httpx"
	"splitfriends/internal/ledger"
	"splitfriends/internal/push"
	"splitfriends/internal/store"
)

var (
	errNotMember   = errors.New("not_member")
	errInvalidDate = errors.New("invalid_date")
)

func validDate(d string) bool {
	_, err := time.Parse("2006-01-02", d)
	return err == nil
}

// buildExpense validates the input against the group and computes shares.
func buildExpense(g ledger.Group, in ledger.ExpenseInput) (ledger.Expense, error) {
	in.Description = strings.TrimSpace(in.Description)
	if in.Description == "" || len(in.Description) > 120 {
		return ledger.Expense{}, errors.New("invalid_description")
	}
	if !validDate(in.Date) {
		return ledger.Expense{}, errInvalidDate
	}
	if in.Amount <= 0 {
		return ledger.Expense{}, ledger.ErrInvalidAmount
	}
	if err := ledger.ValidatePayers(in.Amount, in.Payers); err != nil {
		return ledger.Expense{}, err
	}
	shares, err := ledger.ComputeShares(in.Amount, in.SplitType, in.Shares)
	if err != nil {
		return ledger.Expense{}, err
	}
	members := map[string]bool{}
	for _, m := range g.Members {
		members[m.UserID] = true
	}
	for _, p := range in.Payers {
		if !members[p.UserID] {
			return ledger.Expense{}, errNotMember
		}
	}
	for _, s := range shares {
		if !members[s.UserID] {
			return ledger.Expense{}, errNotMember
		}
	}
	return ledger.Expense{
		GroupID: g.ID, Description: in.Description, Amount: in.Amount, Date: in.Date,
		SplitType: in.SplitType, Notes: strings.TrimSpace(in.Notes), Payers: in.Payers, Shares: shares,
	}, nil
}

// expensePush describes the expense from the recipient's point of view is
// not possible in one message (push is per group), so it states the facts.
func expensePush(g ledger.Group, actor ledger.User, verb string, e ledger.Expense) push.Payload {
	return push.Payload{
		Title: g.Name,
		Body:  actor.Name + " " + verb + " \"" + e.Description + "\" for " + money(g.Currency, e.Amount),
		Tag:   "expense-" + e.ID,
		URL:   "/groups/" + g.ID,
	}
}

func (s *Server) createExpense(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	g := groupFrom(r.Context())
	var in ledger.ExpenseInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	e, err := buildExpense(g, in)
	if err != nil {
		if err.Error() == "invalid_description" {
			httpx.Error(w, 400, "invalid_description", "description must be 1-120 characters")
			return
		}
		handleErr(w, err)
		return
	}
	e.ID = db.NewID()
	e.CreatedBy = u.ID
	e.CreatedAt = db.Now()
	e.UpdatedAt = e.CreatedAt
	_, err = s.commit(r.Context(), g.ID, u, func(tx *sql.Tx) (ledger.Event, push.Payload, error) {
		if err := store.InsertExpense(r.Context(), tx, e); err != nil {
			return ledger.Event{}, push.Payload{}, err
		}
		ev, err := store.AppendEvent(r.Context(), tx, g.ID, ledger.EvExpenseCreated, u, map[string]any{"expense": e})
		return ev, expensePush(g, u, "added", e), err
	})
	if err != nil {
		handleErr(w, err)
		return
	}
	httpx.JSON(w, 201, e)
}

func (s *Server) updateExpense(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	g := groupFrom(r.Context())
	existing, err := store.GetExpense(r.Context(), s.db, g.ID, chi.URLParam(r, "eid"))
	if err != nil {
		handleErr(w, err)
		return
	}
	var in ledger.ExpenseInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	e, err := buildExpense(g, in)
	if err != nil {
		if err.Error() == "invalid_description" {
			httpx.Error(w, 400, "invalid_description", "description must be 1-120 characters")
			return
		}
		handleErr(w, err)
		return
	}
	e.ID = existing.ID
	e.CreatedBy = existing.CreatedBy
	e.CreatedAt = existing.CreatedAt
	e.UpdatedAt = db.Now()
	_, err = s.commit(r.Context(), g.ID, u, func(tx *sql.Tx) (ledger.Event, push.Payload, error) {
		if err := store.UpdateExpense(r.Context(), tx, e); err != nil {
			return ledger.Event{}, push.Payload{}, err
		}
		ev, err := store.AppendEvent(r.Context(), tx, g.ID, ledger.EvExpenseUpdated, u, map[string]any{"expense": e})
		return ev, expensePush(g, u, "updated", e), err
	})
	if err != nil {
		handleErr(w, err)
		return
	}
	httpx.JSON(w, 200, e)
}

func (s *Server) deleteExpense(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	g := groupFrom(r.Context())
	existing, err := store.GetExpense(r.Context(), s.db, g.ID, chi.URLParam(r, "eid"))
	if err != nil {
		handleErr(w, err)
		return
	}
	_, err = s.commit(r.Context(), g.ID, u, func(tx *sql.Tx) (ledger.Event, push.Payload, error) {
		if err := store.SoftDeleteExpense(r.Context(), tx, existing.ID); err != nil {
			return ledger.Event{}, push.Payload{}, err
		}
		ev, err := store.AppendEvent(r.Context(), tx, g.ID, ledger.EvExpenseDeleted, u, map[string]any{
			"expense_id": existing.ID, "description": existing.Description, "amount": existing.Amount,
		})
		return ev, expensePush(g, u, "deleted", existing), err
	})
	if err != nil {
		handleErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) createPayment(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	g := groupFrom(r.Context())
	var in struct {
		FromUserID string `json:"from_user_id"`
		ToUserID   string `json:"to_user_id"`
		Amount     int64  `json:"amount"`
		Date       string `json:"date"`
		Notes      string `json:"notes"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	if in.Amount <= 0 {
		handleErr(w, ledger.ErrInvalidAmount)
		return
	}
	if !validDate(in.Date) {
		handleErr(w, errInvalidDate)
		return
	}
	if in.FromUserID == in.ToUserID {
		httpx.Error(w, 400, "same_user", "payer and payee must differ")
		return
	}
	members := map[string]string{}
	for _, m := range g.Members {
		members[m.UserID] = m.Name
	}
	if _, ok := members[in.FromUserID]; !ok {
		handleErr(w, errNotMember)
		return
	}
	if _, ok := members[in.ToUserID]; !ok {
		handleErr(w, errNotMember)
		return
	}
	p := ledger.Payment{ID: db.NewID(), GroupID: g.ID, FromUserID: in.FromUserID, ToUserID: in.ToUserID,
		Amount: in.Amount, Date: in.Date, Notes: strings.TrimSpace(in.Notes), CreatedBy: u.ID, CreatedAt: db.Now()}
	_, err := s.commit(r.Context(), g.ID, u, func(tx *sql.Tx) (ledger.Event, push.Payload, error) {
		if err := store.InsertPayment(r.Context(), tx, p); err != nil {
			return ledger.Event{}, push.Payload{}, err
		}
		ev, err := store.AppendEvent(r.Context(), tx, g.ID, ledger.EvPaymentCreated, u, map[string]any{"payment": p})
		pl := push.Payload{Title: g.Name, Tag: "payment-" + p.ID, URL: "/groups/" + g.ID,
			Body: members[p.FromUserID] + " paid " + members[p.ToUserID] + " " + money(g.Currency, p.Amount)}
		return ev, pl, err
	})
	if err != nil {
		handleErr(w, err)
		return
	}
	httpx.JSON(w, 201, p)
}

func (s *Server) deletePayment(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	g := groupFrom(r.Context())
	existing, err := store.GetPayment(r.Context(), s.db, g.ID, chi.URLParam(r, "pid"))
	if err != nil {
		handleErr(w, err)
		return
	}
	_, err = s.commit(r.Context(), g.ID, u, func(tx *sql.Tx) (ledger.Event, push.Payload, error) {
		if err := store.SoftDeletePayment(r.Context(), tx, existing.ID); err != nil {
			return ledger.Event{}, push.Payload{}, err
		}
		ev, err := store.AppendEvent(r.Context(), tx, g.ID, ledger.EvPaymentDeleted, u, map[string]any{
			"payment_id": existing.ID, "amount": existing.Amount,
		})
		pl := push.Payload{Title: g.Name, Tag: "payment-" + existing.ID, URL: "/groups/" + g.ID,
			Body: u.Name + " deleted a payment of " + money(g.Currency, existing.Amount)}
		return ev, pl, err
	})
	if err != nil {
		handleErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
