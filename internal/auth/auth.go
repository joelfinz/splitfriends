// Package auth implements passkey (WebAuthn) registration and login plus
// cookie sessions. There are no passwords anywhere in the system.
package auth

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"splitfriends/internal/db"
	"splitfriends/internal/httpx"
	"splitfriends/internal/ledger"
	"splitfriends/internal/store"
)

const (
	sessionCookie   = "sf_session"
	challengeCookie = "sf_wa"
	challengeTTL    = 5 * time.Minute
)

type Service struct {
	db     *db.DB
	wa     *webauthn.WebAuthn
	secure bool

	mu         sync.Mutex
	challenges map[string]pending
}

// pending is the state between a begin and a finish call.
type pending struct {
	session webauthn.SessionData
	userID  string // for register: the id we will create; for add-passkey: the logged-in user
	name    string // display name (register) or key label (add)
	kind    string // "register" | "login" | "add"
	expires time.Time
}

func New(d *db.DB, rpID, appName string, origins []string) (*Service, error) {
	wa, err := webauthn.New(&webauthn.Config{
		RPID:          rpID,
		RPDisplayName: appName,
		RPOrigins:     origins,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementRequired,
			UserVerification: protocol.VerificationPreferred,
		},
		AttestationPreference: protocol.PreferNoAttestation,
		Timeouts: webauthn.TimeoutsConfig{
			Login:        webauthn.TimeoutConfig{Enforce: true, Timeout: 2 * time.Minute, TimeoutUVD: 2 * time.Minute},
			Registration: webauthn.TimeoutConfig{Enforce: true, Timeout: 2 * time.Minute, TimeoutUVD: 2 * time.Minute},
		},
	})
	if err != nil {
		return nil, err
	}
	secure := false
	for _, o := range origins {
		if strings.HasPrefix(o, "https://") {
			secure = true
		}
	}
	s := &Service{db: d, wa: wa, secure: secure, challenges: map[string]pending{}}
	go s.sweep()
	return s, nil
}

func (s *Service) sweep() {
	for range time.Tick(time.Minute) {
		now := time.Now()
		s.mu.Lock()
		for k, p := range s.challenges {
			if now.After(p.expires) {
				delete(s.challenges, k)
			}
		}
		s.mu.Unlock()
	}
}

// ---- webauthn.User adapter ----

type waUser struct {
	user  ledger.User
	creds []webauthn.Credential
}

func (u waUser) WebAuthnID() []byte                         { return []byte(u.user.ID) }
func (u waUser) WebAuthnName() string                       { return u.user.Name }
func (u waUser) WebAuthnDisplayName() string                { return u.user.Name }
func (u waUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

func (s *Service) loadWAUser(ctx context.Context, q store.Q, userID string) (waUser, error) {
	u, err := store.GetUser(ctx, q, userID)
	if err != nil {
		return waUser{}, err
	}
	rows, err := store.CredentialsForUser(ctx, q, userID)
	if err != nil {
		return waUser{}, err
	}
	wu := waUser{user: u}
	for _, c := range rows {
		var cred webauthn.Credential
		if err := json.Unmarshal(c.Data, &cred); err != nil {
			return waUser{}, err
		}
		wu.creds = append(wu.creds, cred)
	}
	return wu, nil
}

func credID(c *webauthn.Credential) string { return base64.RawURLEncoding.EncodeToString(c.ID) }

// ---- challenge store ----

func (s *Service) putPending(w http.ResponseWriter, p pending) {
	tok := db.NewToken()
	p.expires = time.Now().Add(challengeTTL)
	s.mu.Lock()
	s.challenges[tok] = p
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: challengeCookie, Value: tok, Path: "/api/auth", HttpOnly: true,
		Secure: s.secure, SameSite: http.SameSiteLaxMode, MaxAge: int(challengeTTL.Seconds())})
}

func (s *Service) takePending(w http.ResponseWriter, r *http.Request, kind string) (pending, bool) {
	c, err := r.Cookie(challengeCookie)
	if err != nil {
		return pending{}, false
	}
	s.mu.Lock()
	p, ok := s.challenges[c.Value]
	delete(s.challenges, c.Value)
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: challengeCookie, Value: "", Path: "/api/auth", MaxAge: -1})
	if !ok || p.kind != kind || time.Now().After(p.expires) {
		return pending{}, false
	}
	return p, true
}

// ---- sessions ----

func (s *Service) setSession(w http.ResponseWriter, sid string) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: sid, Path: "/", HttpOnly: true,
		Secure: s.secure, SameSite: http.SameSiteLaxMode, MaxAge: int(store.SessionTTL.Seconds())})
}

func (s *Service) clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: s.secure, MaxAge: -1})
}

type ctxKey struct{}

// UserFrom returns the authenticated user placed by Middleware.
func UserFrom(ctx context.Context) (ledger.User, bool) {
	u, ok := ctx.Value(ctxKey{}).(ledger.User)
	return u, ok
}

// Middleware resolves the session cookie to a user if present. It never
// rejects; use Require for protected routes.
func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
			if u, err := store.UserBySession(r.Context(), s.db, c.Value); err == nil {
				r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, u))
			}
		}
		next.ServeHTTP(w, r)
	})
}

func Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := UserFrom(r.Context()); !ok {
			httpx.Error(w, http.StatusUnauthorized, "unauthorized", "sign in required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---- handlers ----

func (s *Service) Me(w http.ResponseWriter, r *http.Request) {
	u, ok := UserFrom(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized", "sign in required")
		return
	}
	httpx.JSON(w, http.StatusOK, u)
}

func (s *Service) UpdateMe(w http.ResponseWriter, r *http.Request) {
	u, _ := UserFrom(r.Context())
	var in struct {
		Name string `json:"name"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 60 {
		httpx.Error(w, 400, "invalid_name", "name must be 1-60 characters")
		return
	}
	if err := store.UpdateUserName(r.Context(), s.db, u.ID, in.Name); err != nil {
		httpx.Internal(w, err)
		return
	}
	u.Name = in.Name
	httpx.JSON(w, 200, u)
}

func (s *Service) RegisterBegin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 60 {
		httpx.Error(w, 400, "invalid_name", "name must be 1-60 characters")
		return
	}
	// The user row is created only when the passkey ceremony finishes.
	wu := waUser{user: ledger.User{ID: db.NewID(), Name: in.Name}}
	creation, sess, err := s.wa.BeginRegistration(wu)
	if err != nil {
		httpx.Internal(w, err)
		return
	}
	s.putPending(w, pending{session: *sess, userID: wu.user.ID, name: in.Name, kind: "register"})
	httpx.JSON(w, 200, creation)
}

func (s *Service) RegisterFinish(w http.ResponseWriter, r *http.Request) {
	p, ok := s.takePending(w, r, "register")
	if !ok {
		httpx.Error(w, 400, "challenge_expired", "registration timed out, please try again")
		return
	}
	wu := waUser{user: ledger.User{ID: p.userID, Name: p.name}}
	cred, err := s.wa.FinishRegistration(wu, p.session, r)
	if err != nil {
		slog.Warn("register finish", "err", err)
		httpx.Error(w, 400, "webauthn_failed", "passkey registration failed")
		return
	}
	data, _ := json.Marshal(cred)
	var user ledger.User
	var sid string
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		var err error
		if user, err = store.CreateUser(r.Context(), tx, p.userID, p.name); err != nil {
			return err
		}
		if err := store.InsertCredential(r.Context(), tx, store.Credential{ID: credID(cred), UserID: user.ID, Name: "First passkey", Data: data}); err != nil {
			return err
		}
		sid, err = store.CreateSession(r.Context(), tx, user.ID)
		return err
	})
	if err != nil {
		httpx.Internal(w, err)
		return
	}
	s.setSession(w, sid)
	httpx.JSON(w, 200, user)
}

func (s *Service) LoginBegin(w http.ResponseWriter, r *http.Request) {
	assertion, sess, err := s.wa.BeginDiscoverableLogin()
	if err != nil {
		httpx.Internal(w, err)
		return
	}
	s.putPending(w, pending{session: *sess, kind: "login"})
	httpx.JSON(w, 200, assertion)
}

func (s *Service) LoginFinish(w http.ResponseWriter, r *http.Request) {
	p, ok := s.takePending(w, r, "login")
	if !ok {
		httpx.Error(w, 400, "challenge_expired", "sign-in timed out, please try again")
		return
	}
	ctx := r.Context()
	var matched store.Credential
	handler := func(rawID, userHandle []byte) (webauthn.User, error) {
		c, err := store.CredentialByID(ctx, s.db, base64.RawURLEncoding.EncodeToString(rawID))
		if err != nil {
			return nil, err
		}
		matched = c
		return s.loadWAUser(ctx, s.db, c.UserID)
	}
	user, cred, err := s.wa.FinishPasskeyLogin(handler, p.session, r)
	if err != nil {
		slog.Warn("login finish", "err", err)
		httpx.Error(w, 401, "webauthn_failed", "passkey sign-in failed")
		return
	}
	data, _ := json.Marshal(cred)
	var sid string
	err = s.db.Tx(ctx, func(tx *sql.Tx) error {
		if err := store.UpdateCredential(ctx, tx, matched.ID, data); err != nil {
			return err
		}
		var err error
		sid, err = store.CreateSession(ctx, tx, matched.UserID)
		return err
	})
	if err != nil {
		httpx.Internal(w, err)
		return
	}
	s.setSession(w, sid)
	httpx.JSON(w, 200, user.(waUser).user)
}

func (s *Service) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		_ = store.DeleteSession(r.Context(), s.db, c.Value)
	}
	s.clearSession(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) ListPasskeys(w http.ResponseWriter, r *http.Request) {
	u, _ := UserFrom(r.Context())
	list, err := store.CredentialsForUser(r.Context(), s.db, u.ID)
	if err != nil {
		httpx.Internal(w, err)
		return
	}
	if list == nil {
		list = []store.Credential{}
	}
	httpx.JSON(w, 200, list)
}

func (s *Service) AddPasskeyBegin(w http.ResponseWriter, r *http.Request) {
	u, _ := UserFrom(r.Context())
	var in struct {
		Name string `json:"name"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	wu, err := s.loadWAUser(r.Context(), s.db, u.ID)
	if err != nil {
		httpx.Internal(w, err)
		return
	}
	var exclude []protocol.CredentialDescriptor
	for _, c := range wu.creds {
		exclude = append(exclude, c.Descriptor())
	}
	creation, sess, err := s.wa.BeginRegistration(wu, webauthn.WithExclusions(exclude))
	if err != nil {
		httpx.Internal(w, err)
		return
	}
	label := strings.TrimSpace(in.Name)
	if label == "" {
		label = "Passkey"
	}
	s.putPending(w, pending{session: *sess, userID: u.ID, name: label, kind: "add"})
	httpx.JSON(w, 200, creation)
}

func (s *Service) AddPasskeyFinish(w http.ResponseWriter, r *http.Request) {
	u, _ := UserFrom(r.Context())
	p, ok := s.takePending(w, r, "add")
	if !ok || p.userID != u.ID {
		httpx.Error(w, 400, "challenge_expired", "timed out, please try again")
		return
	}
	wu, err := s.loadWAUser(r.Context(), s.db, u.ID)
	if err != nil {
		httpx.Internal(w, err)
		return
	}
	cred, err := s.wa.FinishRegistration(wu, p.session, r)
	if err != nil {
		slog.Warn("add passkey finish", "err", err)
		httpx.Error(w, 400, "webauthn_failed", "passkey registration failed")
		return
	}
	data, _ := json.Marshal(cred)
	c := store.Credential{ID: credID(cred), UserID: u.ID, Name: p.name, Data: data, CreatedAt: db.Now()}
	if err := store.InsertCredential(r.Context(), s.db, c); err != nil {
		httpx.Internal(w, err)
		return
	}
	httpx.JSON(w, 201, c)
}

func (s *Service) DeletePasskey(w http.ResponseWriter, r *http.Request, id string) {
	u, _ := UserFrom(r.Context())
	list, err := store.CredentialsForUser(r.Context(), s.db, u.ID)
	if err != nil {
		httpx.Internal(w, err)
		return
	}
	if len(list) <= 1 {
		httpx.Error(w, 400, "last_passkey", "you cannot remove your only passkey")
		return
	}
	if err := store.DeleteCredential(r.Context(), s.db, u.ID, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpx.Error(w, 404, "not_found", "passkey not found")
			return
		}
		httpx.Internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
