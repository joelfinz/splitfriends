package store

import (
	"context"
	"encoding/json"
	"time"

	"splitfriends/internal/ledger"
)

// Read models for the admin dashboard.

type AdminUserRow struct {
	ledger.User
	Passkeys   int
	Groups     int
	Sessions   int
	LastSeenAt string
	LastIP     string
	Actions30d int
}

func AdminUsers(ctx context.Context, q Q) ([]AdminUserRow, error) {
	since := time.Now().UTC().Add(-30 * 24 * time.Hour).Format(time.RFC3339)
	rows, err := q.QueryContext(ctx, `
SELECT u.id, u.name, u.created_at,
  (SELECT COUNT(*) FROM credentials c WHERE c.user_id=u.id),
  (SELECT COUNT(*) FROM members m WHERE m.user_id=u.id),
  (SELECT COUNT(*) FROM sessions s WHERE s.user_id=u.id AND s.expires_at > ?),
  COALESCE((SELECT MAX(s.last_seen_at) FROM sessions s WHERE s.user_id=u.id), ''),
  COALESCE((SELECT s.ip FROM sessions s WHERE s.user_id=u.id ORDER BY s.last_seen_at DESC LIMIT 1), ''),
  (SELECT COUNT(*) FROM events e WHERE e.actor_id=u.id AND e.created_at > ?)
FROM users u ORDER BY 7 DESC, u.created_at DESC`, time.Now().UTC().Format(time.RFC3339), since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AdminUserRow
	for rows.Next() {
		var r AdminUserRow
		if err := rows.Scan(&r.ID, &r.Name, &r.CreatedAt, &r.Passkeys, &r.Groups, &r.Sessions, &r.LastSeenAt, &r.LastIP, &r.Actions30d); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type SessionRow struct {
	ID         string
	CreatedAt  string
	ExpiresAt  string
	IP         string
	UserAgent  string
	LastSeenAt string
}

func SessionsForUser(ctx context.Context, q Q, userID string) ([]SessionRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT id,created_at,expires_at,ip,user_agent,last_seen_at FROM sessions WHERE user_id=? AND expires_at > ? ORDER BY last_seen_at DESC`, userID, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SessionRow
	for rows.Next() {
		var s SessionRow
		if err := rows.Scan(&s.ID, &s.CreatedAt, &s.ExpiresAt, &s.IP, &s.UserAgent, &s.LastSeenAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// DeleteUserSession revokes one session, scoped to the user to avoid mix-ups.
func DeleteUserSession(ctx context.Context, q Q, userID, sid string) (bool, error) {
	res, err := q.ExecContext(ctx, `DELETE FROM sessions WHERE id=? AND user_id=?`, sid, userID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

type UserGroupRow struct {
	ledger.Group
	MemberCount int
}

func GroupsWithCountsForUser(ctx context.Context, q Q, userID string) ([]UserGroupRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT g.id,g.name,g.currency,g.created_by,g.created_at,g.last_seq,(SELECT COUNT(*) FROM members x WHERE x.group_id=g.id)
		FROM groups g JOIN members m ON m.group_id=g.id WHERE m.user_id=? ORDER BY g.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UserGroupRow
	for rows.Next() {
		var r UserGroupRow
		if err := rows.Scan(&r.ID, &r.Name, &r.Currency, &r.CreatedBy, &r.CreatedAt, &r.LastSeq, &r.MemberCount); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// EventRow is an event with its IP and group name, for admin timelines.
type EventRow struct {
	ledger.Event
	GroupName string
	IP        string
}

const adminEventCols = `e.id,e.group_id,e.seq,e.type,e.actor_id,u.name,e.payload,e.created_at,e.ip,g.name
	FROM events e JOIN users u ON u.id=e.actor_id JOIN groups g ON g.id=e.group_id`

func scanEventRows(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
	Close() error
}) ([]EventRow, error) {
	defer rows.Close()
	var out []EventRow
	for rows.Next() {
		var r EventRow
		var raw string
		if err := rows.Scan(&r.ID, &r.GroupID, &r.Seq, &r.Type, &r.ActorID, &r.ActorName, &raw, &r.CreatedAt, &r.IP, &r.GroupName); err != nil {
			return nil, err
		}
		r.Payload = json.RawMessage(raw)
		out = append(out, r)
	}
	return out, rows.Err()
}

func EventsByActor(ctx context.Context, q Q, userID string, limit int) ([]EventRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+adminEventCols+` WHERE e.actor_id=? ORDER BY e.id DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	return scanEventRows(rows)
}

func EventsRecent(ctx context.Context, q Q, limit int) ([]EventRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+adminEventCols+` ORDER BY e.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	return scanEventRows(rows)
}

type Overview struct {
	Users, Groups, Expenses, Payments, ActiveSessions, PushSubs int
	Active24h                                                   int
}

func AdminOverview(ctx context.Context, q Q) (Overview, error) {
	var o Overview
	now := time.Now().UTC().Format(time.RFC3339)
	day := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	err := q.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM users),
		(SELECT COUNT(*) FROM groups),
		(SELECT COUNT(*) FROM expenses WHERE deleted_at IS NULL),
		(SELECT COUNT(*) FROM payments WHERE deleted_at IS NULL),
		(SELECT COUNT(*) FROM sessions WHERE expires_at > ?),
		(SELECT COUNT(*) FROM push_subscriptions),
		(SELECT COUNT(DISTINCT user_id) FROM sessions WHERE last_seen_at > ?)`, now, day).
		Scan(&o.Users, &o.Groups, &o.Expenses, &o.Payments, &o.ActiveSessions, &o.PushSubs, &o.Active24h)
	return o, err
}
