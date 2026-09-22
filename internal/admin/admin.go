// Package admin is a small server-rendered dashboard mounted at /admin. It is
// deliberately separate from the SPA and the passkey system: nothing here is
// reachable or referenced from the app bundle, and it only exists when an
// admin password is configured.
package admin

import (
	"context"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"splitfriends/internal/config"
	"splitfriends/internal/db"
	"splitfriends/internal/store"
)

//go:embed templates/*.html
var tplFS embed.FS

const (
	cookieName    = "sf_admin"
	sessionTTL    = 12 * time.Hour
	maxFailures   = 5
	failureWindow = 15 * time.Minute
)

type Handler struct {
	cfg    config.Config
	db     *db.DB
	tpl    *template.Template
	secure bool

	mu       sync.Mutex
	sessions map[string]time.Time // token -> expiry
}

// New returns nil when no admin password is configured, so callers can skip
// mounting the routes entirely.
func New(cfg config.Config, d *db.DB) *Handler {
	if cfg.AdminPassword == "" {
		return nil
	}
	funcs := template.FuncMap{
		"ago":      ago,
		"when":     when,
		"device":   device,
		"describe": describe,
		"money":    moneyFmt,
	}
	tpl := template.Must(template.New("").Funcs(funcs).ParseFS(tplFS, "templates/*.html"))
	secure := false
	for _, o := range cfg.Origins {
		if strings.HasPrefix(o, "https://") {
			secure = true
		}
	}
	h := &Handler{cfg: cfg, db: d, tpl: tpl, secure: secure, sessions: map[string]time.Time{}}
	go h.housekeeping()
	return h
}

// housekeeping purges expired admin tokens, old audit rows and dead sessions daily.
func (h *Handler) housekeeping() {
	run := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if n, err := store.PurgeAudit(ctx, h.db, store.AuditRetentionPeriod); err == nil && n > 0 {
			slog.Info("purged audit rows", "n", n)
		}
		if n, err := store.PurgeExpiredSessions(ctx, h.db); err == nil && n > 0 {
			slog.Info("purged expired sessions", "n", n)
		}
		h.mu.Lock()
		now := time.Now()
		for k, exp := range h.sessions {
			if now.After(exp) {
				delete(h.sessions, k)
			}
		}
		h.mu.Unlock()
	}
	run()
	for range time.Tick(24 * time.Hour) {
		run()
	}
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hd := w.Header()
			hd.Set("X-Robots-Tag", "noindex, nofollow")
			hd.Set("Cache-Control", "no-store")
			hd.Set("X-Frame-Options", "DENY")
			// same-origin (not no-referrer): with no-referrer browsers send
			// "Origin: null" on form posts, which would defeat the CSRF check.
			hd.Set("Referrer-Policy", "same-origin")
			hd.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src 'self' data:; form-action 'self'")
			next.ServeHTTP(w, r)
		})
	})
	r.Get("/", h.overview)
	r.Post("/login", h.login)
	r.Post("/logout", h.logout)
	r.Get("/users", h.users)
	r.Get("/users/{id}", h.user)
	r.Post("/users/{id}/sessions/{sid}/revoke", h.revoke)
	r.NotFound(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/admin", http.StatusFound) })
	return r
}

// ---- auth ----

func (h *Handler) authed(r *http.Request) bool {
	c, err := r.Cookie(cookieName)
	if err != nil || c.Value == "" {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	exp, ok := h.sessions[c.Value]
	if !ok || time.Now().After(exp) {
		delete(h.sessions, c.Value)
		return false
	}
	return true
}

// sameOrigin is the CSRF check for POSTs: the browser must send an Origin
// (or Referer) that matches one of the configured origins or the Host.
func (h *Handler) sameOrigin(r *http.Request) bool {
	src := r.Header.Get("Origin")
	if src == "" {
		src = r.Header.Get("Referer")
	}
	if src == "" || src == "null" {
		return false
	}
	u, err := url.Parse(src)
	if err != nil || u.Host == "" {
		return false
	}
	for _, o := range h.cfg.Origins {
		if strings.EqualFold(u.Scheme+"://"+u.Host, o) {
			return true
		}
	}
	return strings.EqualFold(u.Host, r.Host)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	if !h.sameOrigin(r) {
		h.render(w, "login.html", map[string]any{"Error": "Request rejected: origin check failed. Reload the page and try again."}, http.StatusForbidden)
		return
	}
	ip := store.IPFrom(r.Context())
	if n, err := store.FailedAdminLogins(r.Context(), h.db, ip, failureWindow); err == nil && n >= maxFailures {
		h.render(w, "login.html", map[string]any{"Error": "Too many attempts. Try again later."}, http.StatusTooManyRequests)
		return
	}
	_ = r.ParseForm()
	pw := r.PostFormValue("password")
	if subtle.ConstantTimeCompare([]byte(pw), []byte(h.cfg.AdminPassword)) != 1 {
		_ = store.Audit(r.Context(), h.db, "", store.AuditAdminLoginFail, "", ip, r.UserAgent())
		time.Sleep(400 * time.Millisecond)
		h.render(w, "login.html", map[string]any{"Error": "Wrong password."}, http.StatusUnauthorized)
		return
	}
	tok := db.NewToken()
	h.mu.Lock()
	h.sessions[tok] = time.Now().Add(sessionTTL)
	h.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: tok, Path: "/admin", HttpOnly: true, Secure: h.secure,
		SameSite: http.SameSiteStrictMode, MaxAge: int(sessionTTL.Seconds())})
	_ = store.Audit(r.Context(), h.db, "", store.AuditAdminLogin, "", ip, r.UserAgent())
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		h.mu.Lock()
		delete(h.sessions, c.Value)
		h.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/admin", HttpOnly: true, Secure: h.secure, MaxAge: -1})
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

// guard renders the login page when unauthenticated and reports whether to continue.
func (h *Handler) guard(w http.ResponseWriter, r *http.Request) bool {
	if h.authed(r) {
		return true
	}
	h.render(w, "login.html", map[string]any{}, http.StatusOK)
	return false
}

func (h *Handler) render(w http.ResponseWriter, name string, data map[string]any, status int) {
	data["AppName"] = h.cfg.AppName
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := h.tpl.ExecuteTemplate(w, name, data); err != nil {
		slog.Error("admin template", "name", name, "err", err)
	}
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	slog.Error("admin", "err", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}

// ---- pages ----

func (h *Handler) overview(w http.ResponseWriter, r *http.Request) {
	if !h.guard(w, r) {
		return
	}
	ctx := r.Context()
	ov, err := store.AdminOverview(ctx, h.db)
	if err != nil {
		h.fail(w, err)
		return
	}
	events, err := store.EventsRecent(ctx, h.db, 50)
	if err != nil {
		h.fail(w, err)
		return
	}
	audit, err := store.AuditRecent(ctx, h.db, 30)
	if err != nil {
		h.fail(w, err)
		return
	}
	h.render(w, "overview.html", map[string]any{"Page": "overview", "Overview": ov, "Events": events, "Audit": audit}, 200)
}

func (h *Handler) users(w http.ResponseWriter, r *http.Request) {
	if !h.guard(w, r) {
		return
	}
	rows, err := store.AdminUsers(r.Context(), h.db)
	if err != nil {
		h.fail(w, err)
		return
	}
	h.render(w, "users.html", map[string]any{"Page": "users", "Users": rows}, 200)
}

// timelineItem merges audit entries and ledger events for one user.
type timelineItem struct {
	At     string
	Kind   string
	Text   string
	Group  string
	IP     string
	Device string
}

func (h *Handler) user(w http.ResponseWriter, r *http.Request) {
	if !h.guard(w, r) {
		return
	}
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	u, err := store.GetUser(ctx, h.db, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	sessions, err := store.SessionsForUser(ctx, h.db, id)
	if err != nil {
		h.fail(w, err)
		return
	}
	groups, err := store.GroupsWithCountsForUser(ctx, h.db, id)
	if err != nil {
		h.fail(w, err)
		return
	}
	creds, err := store.CredentialsForUser(ctx, h.db, id)
	if err != nil {
		h.fail(w, err)
		return
	}
	events, err := store.EventsByActor(ctx, h.db, id, 100)
	if err != nil {
		h.fail(w, err)
		return
	}
	audit, err := store.AuditForUser(ctx, h.db, id, 100)
	if err != nil {
		h.fail(w, err)
		return
	}
	var tl []timelineItem
	for _, e := range events {
		tl = append(tl, timelineItem{At: e.CreatedAt, Kind: e.Type, Text: describe(e), Group: e.GroupName, IP: e.IP})
	}
	for _, a := range audit {
		tl = append(tl, timelineItem{At: a.CreatedAt, Kind: a.Kind, Text: auditText(a), IP: a.IP, Device: device(a.UserAgent)})
	}
	sort.Slice(tl, func(i, j int) bool { return tl[i].At > tl[j].At })
	if len(tl) > 150 {
		tl = tl[:150]
	}
	h.render(w, "user.html", map[string]any{"Page": "users", "User": u, "Sessions": sessions, "Groups": groups,
		"Passkeys": creds, "Timeline": tl, "Flash": r.URL.Query().Get("flash")}, 200)
}

func (h *Handler) revoke(w http.ResponseWriter, r *http.Request) {
	if !h.authed(r) {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	if !h.sameOrigin(r) {
		http.Redirect(w, r, "/admin/users/"+url.PathEscape(chi.URLParam(r, "id"))+"?flash=Request+rejected+by+origin+check", http.StatusSeeOther)
		return
	}
	uid, sid := chi.URLParam(r, "id"), chi.URLParam(r, "sid")
	ok, err := store.DeleteUserSession(r.Context(), h.db, uid, sid)
	if err != nil {
		h.fail(w, err)
		return
	}
	if ok {
		_ = store.Audit(r.Context(), h.db, uid, store.AuditSessionRevoked, shortID(sid), store.IPFrom(r.Context()), r.UserAgent())
	}
	http.Redirect(w, r, "/admin/users/"+url.PathEscape(uid)+"?flash=Session+revoked", http.StatusSeeOther)
}

func shortID(s string) string {
	if len(s) > 8 {
		return s[:8] + "…"
	}
	return s
}

// ---- template helpers ----

func parseT(s string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, s)
	return t, err == nil
}

func when(s string) string {
	t, ok := parseT(s)
	if !ok {
		return s
	}
	return t.Local().Format("2006-01-02 15:04")
}

func ago(s string) string {
	t, ok := parseT(s)
	if !ok || s == "" {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Local().Format("2006-01-02")
	}
}

// device turns a user agent into "Safari · iPhone" style text.
func device(ua string) string {
	if ua == "" {
		return "unknown"
	}
	os := "other"
	switch {
	case strings.Contains(ua, "iPhone"):
		os = "iPhone"
	case strings.Contains(ua, "iPad"):
		os = "iPad"
	case strings.Contains(ua, "Android"):
		os = "Android"
	case strings.Contains(ua, "Macintosh"):
		os = "Mac"
	case strings.Contains(ua, "Windows"):
		os = "Windows"
	case strings.Contains(ua, "Linux"):
		os = "Linux"
	}
	br := "browser"
	switch {
	case strings.Contains(ua, "Edg/"):
		br = "Edge"
	case strings.Contains(ua, "OPR/"):
		br = "Opera"
	case strings.Contains(ua, "Firefox/"):
		br = "Firefox"
	case strings.Contains(ua, "Chrome/") || strings.Contains(ua, "CriOS/"):
		br = "Chrome"
	case strings.Contains(ua, "Safari/"):
		br = "Safari"
	case strings.HasPrefix(ua, "curl/"):
		br = "curl"
	}
	return br + " · " + os
}

func moneyFmt(currency string, minor int64) string {
	neg := minor < 0
	if neg {
		minor = -minor
	}
	s := fmt.Sprintf("%d.%02d", minor/100, minor%100)
	if neg {
		s = "-" + s
	}
	return currency + " " + s
}

// describe summarises an event payload for a human reader.
func describe(e store.EventRow) string {
	raw, _ := e.Payload.(json.RawMessage)
	var p struct {
		Expense struct {
			Description string `json:"description"`
			Amount      int64  `json:"amount"`
		} `json:"expense"`
		Payment struct {
			Amount int64 `json:"amount"`
		} `json:"payment"`
		Member struct {
			Name string `json:"name"`
		} `json:"member"`
		Group struct {
			Name     string `json:"name"`
			Currency string `json:"currency"`
		} `json:"group"`
		Description string `json:"description"`
		Amount      int64  `json:"amount"`
	}
	_ = json.Unmarshal(raw, &p)
	verb := strings.TrimPrefix(e.Type, "expense.")
	verb = strings.TrimPrefix(verb, "payment.")
	switch {
	case strings.HasPrefix(e.Type, "expense."):
		desc, amt := p.Expense.Description, p.Expense.Amount
		if desc == "" {
			desc, amt = p.Description, p.Amount
		}
		return fmt.Sprintf("%s expense “%s” (%d)", verb, desc, amt)
	case strings.HasPrefix(e.Type, "payment."):
		amt := p.Payment.Amount
		if amt == 0 {
			amt = p.Amount
		}
		return fmt.Sprintf("%s payment (%d)", verb, amt)
	case e.Type == "member.joined":
		return p.Member.Name + " joined"
	case e.Type == "member.left":
		return p.Member.Name + " left"
	case e.Type == "group.created":
		return "created group “" + p.Group.Name + "” (" + p.Group.Currency + ")"
	case e.Type == "group.updated":
		return "updated group “" + p.Group.Name + "”"
	}
	return e.Type
}

func auditText(a store.AuditEntry) string {
	switch a.Kind {
	case store.AuditRegister:
		return "registered"
	case store.AuditLogin:
		return "signed in (" + a.Detail + ")"
	case store.AuditLogout:
		return "signed out"
	case store.AuditPasskeyAdded:
		return "added passkey “" + a.Detail + "”"
	case store.AuditPasskeyRemoved:
		return "removed a passkey"
	case store.AuditSessionRevoked:
		return "session revoked by admin"
	case store.AuditAdminLogin:
		return "admin signed in"
	case store.AuditAdminLoginFail:
		return "failed admin login"
	}
	return a.Kind
}
