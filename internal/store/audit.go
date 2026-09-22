package store

import (
	"context"
	"time"

	"splitfriends/internal/db"
)

// ipKey carries the client IP through context so store calls can record it
// without every handler threading it by hand.
type ipKey struct{}

func WithIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, ipKey{}, ip)
}

func IPFrom(ctx context.Context) string {
	ip, _ := ctx.Value(ipKey{}).(string)
	return ip
}

// Audit kinds. Mutations to ledger data live in the events table; this log
// covers authentication and admin actions.
const (
	AuditRegister        = "auth.register"
	AuditLogin           = "auth.login"
	AuditLogout          = "auth.logout"
	AuditPasskeyAdded    = "passkey.added"
	AuditPasskeyRemoved  = "passkey.removed"
	AuditAdminLogin      = "admin.login"
	AuditAdminLoginFail  = "admin.login_failed"
	AuditSessionRevoked  = "admin.session_revoked"
	AuditRetentionPeriod = 90 * 24 * time.Hour
)

type AuditEntry struct {
	ID        int64
	UserID    string
	UserName  string
	Kind      string
	Detail    string
	IP        string
	UserAgent string
	CreatedAt string
}

// Audit writes one entry. userID may be empty.
func Audit(ctx context.Context, q Q, userID, kind, detail, ip, userAgent string) error {
	var uid any
	if userID != "" {
		uid = userID
	}
	_, err := q.ExecContext(ctx, `INSERT INTO audit_log(user_id,kind,detail,ip,user_agent,created_at) VALUES(?,?,?,?,?,?)`,
		uid, kind, detail, ip, userAgent, db.Now())
	return err
}

func PurgeAudit(ctx context.Context, q Q, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-olderThan).Format(time.RFC3339)
	res, err := q.ExecContext(ctx, `DELETE FROM audit_log WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func PurgeExpiredSessions(ctx context.Context, q Q) (int64, error) {
	res, err := q.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, db.Now())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func scanAudit(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
	Close() error
}) ([]AuditEntry, error) {
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var a AuditEntry
		if err := rows.Scan(&a.ID, &a.UserID, &a.UserName, &a.Kind, &a.Detail, &a.IP, &a.UserAgent, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

const auditCols = `a.id,COALESCE(a.user_id,''),COALESCE(u.name,''),a.kind,a.detail,a.ip,a.user_agent,a.created_at FROM audit_log a LEFT JOIN users u ON u.id=a.user_id`

func AuditForUser(ctx context.Context, q Q, userID string, limit int) ([]AuditEntry, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+auditCols+` WHERE a.user_id=? ORDER BY a.id DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	return scanAudit(rows)
}

func AuditRecent(ctx context.Context, q Q, limit int) ([]AuditEntry, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+auditCols+` ORDER BY a.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	return scanAudit(rows)
}

// FailedAdminLogins counts failures from an IP inside the window (rate limiting survives restarts).
func FailedAdminLogins(ctx context.Context, q Q, ip string, window time.Duration) (int, error) {
	var n int
	since := time.Now().UTC().Add(-window).Format(time.RFC3339)
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_log WHERE kind=? AND ip=? AND created_at > ?`, AuditAdminLoginFail, ip, since).Scan(&n)
	return n, err
}
