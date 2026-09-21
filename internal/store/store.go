// Package store is the only place that speaks SQL. Handlers compose these
// calls inside a transaction so an append to the event log and the state it
// describes always commit together.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"splitfriends/internal/db"
	"splitfriends/internal/ledger"
)

var ErrNotFound = errors.New("not_found")

// Q is satisfied by both *sql.Tx and *sql.DB.
type Q interface {
	ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
}

func noRows(err error) bool { return errors.Is(err, sql.ErrNoRows) }

// ---- users ----

func CreateUser(ctx context.Context, q Q, id, name string) (ledger.User, error) {
	u := ledger.User{ID: id, Name: name, CreatedAt: db.Now()}
	_, err := q.ExecContext(ctx, `INSERT INTO users(id,name,created_at) VALUES(?,?,?)`, u.ID, u.Name, u.CreatedAt)
	return u, err
}

func GetUser(ctx context.Context, q Q, id string) (ledger.User, error) {
	var u ledger.User
	err := q.QueryRowContext(ctx, `SELECT id,name,created_at FROM users WHERE id=?`, id).Scan(&u.ID, &u.Name, &u.CreatedAt)
	if noRows(err) {
		return u, ErrNotFound
	}
	return u, err
}

func UpdateUserName(ctx context.Context, q Q, id, name string) error {
	_, err := q.ExecContext(ctx, `UPDATE users SET name=? WHERE id=?`, name, id)
	return err
}

// ---- credentials ----

type Credential struct {
	ID         string `json:"id"`
	UserID     string `json:"-"`
	Name       string `json:"name"`
	Data       []byte `json:"-"`
	CreatedAt  string `json:"created_at"`
	LastUsedAt string `json:"last_used_at,omitempty"`
}

func InsertCredential(ctx context.Context, q Q, c Credential) error {
	_, err := q.ExecContext(ctx, `INSERT INTO credentials(id,user_id,name,data,created_at) VALUES(?,?,?,?,?)`,
		c.ID, c.UserID, c.Name, c.Data, db.Now())
	return err
}

func UpdateCredential(ctx context.Context, q Q, id string, data []byte) error {
	_, err := q.ExecContext(ctx, `UPDATE credentials SET data=?, last_used_at=? WHERE id=?`, data, db.Now(), id)
	return err
}

func CredentialsForUser(ctx context.Context, q Q, userID string) ([]Credential, error) {
	rows, err := q.QueryContext(ctx, `SELECT id,user_id,name,data,created_at,COALESCE(last_used_at,'') FROM credentials WHERE user_id=? ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Credential
	for rows.Next() {
		var c Credential
		if err := rows.Scan(&c.ID, &c.UserID, &c.Name, &c.Data, &c.CreatedAt, &c.LastUsedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func CredentialByID(ctx context.Context, q Q, id string) (Credential, error) {
	var c Credential
	err := q.QueryRowContext(ctx, `SELECT id,user_id,name,data,created_at,COALESCE(last_used_at,'') FROM credentials WHERE id=?`, id).
		Scan(&c.ID, &c.UserID, &c.Name, &c.Data, &c.CreatedAt, &c.LastUsedAt)
	if noRows(err) {
		return c, ErrNotFound
	}
	return c, err
}

func DeleteCredential(ctx context.Context, q Q, userID, id string) error {
	res, err := q.ExecContext(ctx, `DELETE FROM credentials WHERE id=? AND user_id=?`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- sessions ----

const SessionTTL = 90 * 24 * time.Hour

func CreateSession(ctx context.Context, q Q, userID string) (string, error) {
	id := db.NewToken()
	now := time.Now().UTC()
	_, err := q.ExecContext(ctx, `INSERT INTO sessions(id,user_id,created_at,expires_at) VALUES(?,?,?,?)`,
		id, userID, now.Format(time.RFC3339), now.Add(SessionTTL).Format(time.RFC3339))
	return id, err
}

// UserBySession returns the user for a live session, or ErrNotFound.
func UserBySession(ctx context.Context, q Q, sid string) (ledger.User, error) {
	var u ledger.User
	err := q.QueryRowContext(ctx, `SELECT u.id,u.name,u.created_at FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.id=? AND s.expires_at > ?`,
		sid, db.Now()).Scan(&u.ID, &u.Name, &u.CreatedAt)
	if noRows(err) {
		return u, ErrNotFound
	}
	return u, err
}

func DeleteSession(ctx context.Context, q Q, sid string) error {
	_, err := q.ExecContext(ctx, `DELETE FROM sessions WHERE id=?`, sid)
	return err
}

// ---- groups & members ----

func CreateGroup(ctx context.Context, q Q, name, currency, creator string) (ledger.Group, error) {
	g := ledger.Group{ID: db.NewID(), Name: name, Currency: currency, CreatedBy: creator, CreatedAt: db.Now()}
	if _, err := q.ExecContext(ctx, `INSERT INTO groups(id,name,currency,created_by,created_at) VALUES(?,?,?,?,?)`,
		g.ID, g.Name, g.Currency, g.CreatedBy, g.CreatedAt); err != nil {
		return g, err
	}
	return g, nil
}

func UpdateGroup(ctx context.Context, q Q, id, name, currency string) error {
	_, err := q.ExecContext(ctx, `UPDATE groups SET name=?, currency=? WHERE id=?`, name, currency, id)
	return err
}

// GetGroup loads a group with its members (no balance).
func GetGroup(ctx context.Context, q Q, id string) (ledger.Group, error) {
	var g ledger.Group
	err := q.QueryRowContext(ctx, `SELECT id,name,currency,created_by,created_at,last_seq FROM groups WHERE id=?`, id).
		Scan(&g.ID, &g.Name, &g.Currency, &g.CreatedBy, &g.CreatedAt, &g.LastSeq)
	if noRows(err) {
		return g, ErrNotFound
	}
	if err != nil {
		return g, err
	}
	g.Members, err = Members(ctx, q, id)
	return g, err
}

func Members(ctx context.Context, q Q, groupID string) ([]ledger.Member, error) {
	rows, err := q.QueryContext(ctx, `SELECT m.user_id,u.name,m.joined_at FROM members m JOIN users u ON u.id=m.user_id WHERE m.group_id=? ORDER BY m.joined_at`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ledger.Member{}
	for rows.Next() {
		var m ledger.Member
		if err := rows.Scan(&m.UserID, &m.Name, &m.JoinedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func MemberIDs(ctx context.Context, q Q, groupID string) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT user_id FROM members WHERE group_id=?`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func IsMember(ctx context.Context, q Q, groupID, userID string) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM members WHERE group_id=? AND user_id=?`, groupID, userID).Scan(&n)
	return n > 0, err
}

func AddMember(ctx context.Context, q Q, groupID, userID string) (ledger.Member, error) {
	u, err := GetUser(ctx, q, userID)
	if err != nil {
		return ledger.Member{}, err
	}
	m := ledger.Member{UserID: userID, Name: u.Name, JoinedAt: db.Now()}
	_, err = q.ExecContext(ctx, `INSERT INTO members(group_id,user_id,joined_at) VALUES(?,?,?)`, groupID, userID, m.JoinedAt)
	return m, err
}

func RemoveMember(ctx context.Context, q Q, groupID, userID string) error {
	_, err := q.ExecContext(ctx, `DELETE FROM members WHERE group_id=? AND user_id=?`, groupID, userID)
	return err
}

func GroupIDsForUser(ctx context.Context, q Q, userID string) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT group_id FROM members WHERE user_id=?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// GroupsForUser returns every group the user belongs to, with members and
// the user's own net balance computed from the ledger.
func GroupsForUser(ctx context.Context, q Q, userID string) ([]ledger.Group, error) {
	ids, err := GroupIDsForUser(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	out := make([]ledger.Group, 0, len(ids))
	for _, id := range ids {
		g, err := GetGroup(ctx, q, id)
		if err != nil {
			return nil, err
		}
		exps, err := Expenses(ctx, q, id)
		if err != nil {
			return nil, err
		}
		pays, err := Payments(ctx, q, id)
		if err != nil {
			return nil, err
		}
		for _, b := range ledger.Balances(memberIDs(g.Members), exps, pays) {
			if b.UserID == userID {
				g.MyBalance = b.Net
			}
		}
		out = append(out, g)
	}
	return out, nil
}

func memberIDs(ms []ledger.Member) []string {
	ids := make([]string, len(ms))
	for i, m := range ms {
		ids[i] = m.UserID
	}
	return ids
}

// ---- invites ----

type Invite struct {
	Token     string
	GroupID   string
	CreatedBy string
	CreatedAt string
	ExpiresAt string
}

func CreateInvite(ctx context.Context, q Q, groupID, userID string, ttl time.Duration) (Invite, error) {
	now := time.Now().UTC()
	inv := Invite{Token: db.NewToken(), GroupID: groupID, CreatedBy: userID,
		CreatedAt: now.Format(time.RFC3339), ExpiresAt: now.Add(ttl).Format(time.RFC3339)}
	_, err := q.ExecContext(ctx, `INSERT INTO invites(token,group_id,created_by,created_at,expires_at) VALUES(?,?,?,?,?)`,
		inv.Token, inv.GroupID, inv.CreatedBy, inv.CreatedAt, inv.ExpiresAt)
	return inv, err
}

func GetInvite(ctx context.Context, q Q, token string) (Invite, error) {
	var inv Invite
	err := q.QueryRowContext(ctx, `SELECT token,group_id,created_by,created_at,expires_at FROM invites WHERE token=? AND expires_at > ?`, token, db.Now()).
		Scan(&inv.Token, &inv.GroupID, &inv.CreatedBy, &inv.CreatedAt, &inv.ExpiresAt)
	if noRows(err) {
		return inv, ErrNotFound
	}
	return inv, err
}

// ---- expenses ----

func InsertExpense(ctx context.Context, q Q, e ledger.Expense) error {
	if _, err := q.ExecContext(ctx, `INSERT INTO expenses(id,group_id,description,amount,date,split_type,notes,created_by,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)`, e.ID, e.GroupID, e.Description, e.Amount, e.Date, e.SplitType, e.Notes, e.CreatedBy, e.CreatedAt, e.UpdatedAt); err != nil {
		return err
	}
	return writeExpenseLines(ctx, q, e)
}

func UpdateExpense(ctx context.Context, q Q, e ledger.Expense) error {
	if _, err := q.ExecContext(ctx, `UPDATE expenses SET description=?,amount=?,date=?,split_type=?,notes=?,updated_at=? WHERE id=?`,
		e.Description, e.Amount, e.Date, e.SplitType, e.Notes, e.UpdatedAt, e.ID); err != nil {
		return err
	}
	if _, err := q.ExecContext(ctx, `DELETE FROM expense_payers WHERE expense_id=?`, e.ID); err != nil {
		return err
	}
	if _, err := q.ExecContext(ctx, `DELETE FROM expense_shares WHERE expense_id=?`, e.ID); err != nil {
		return err
	}
	return writeExpenseLines(ctx, q, e)
}

func writeExpenseLines(ctx context.Context, q Q, e ledger.Expense) error {
	for _, p := range e.Payers {
		if _, err := q.ExecContext(ctx, `INSERT INTO expense_payers(expense_id,user_id,amount) VALUES(?,?,?)`, e.ID, p.UserID, p.Amount); err != nil {
			return err
		}
	}
	for _, s := range e.Shares {
		if _, err := q.ExecContext(ctx, `INSERT INTO expense_shares(expense_id,user_id,value,amount) VALUES(?,?,?,?)`, e.ID, s.UserID, s.Value, s.Amount); err != nil {
			return err
		}
	}
	return nil
}

func SoftDeleteExpense(ctx context.Context, q Q, id string) error {
	_, err := q.ExecContext(ctx, `UPDATE expenses SET deleted_at=? WHERE id=? AND deleted_at IS NULL`, db.Now(), id)
	return err
}

func GetExpense(ctx context.Context, q Q, groupID, id string) (ledger.Expense, error) {
	list, err := queryExpenses(ctx, q, `WHERE e.group_id=? AND e.id=? AND e.deleted_at IS NULL`, groupID, id)
	if err != nil {
		return ledger.Expense{}, err
	}
	if len(list) == 0 {
		return ledger.Expense{}, ErrNotFound
	}
	return list[0], nil
}

func Expenses(ctx context.Context, q Q, groupID string) ([]ledger.Expense, error) {
	return queryExpenses(ctx, q, `WHERE e.group_id=? AND e.deleted_at IS NULL`, groupID)
}

func queryExpenses(ctx context.Context, q Q, where string, args ...any) ([]ledger.Expense, error) {
	rows, err := q.QueryContext(ctx, `SELECT e.id,e.group_id,e.description,e.amount,e.date,e.split_type,e.notes,e.created_by,e.created_at,e.updated_at
		FROM expenses e `+where+` ORDER BY e.date DESC, e.created_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	out := []ledger.Expense{}
	idx := map[string]int{}
	for rows.Next() {
		var e ledger.Expense
		if err := rows.Scan(&e.ID, &e.GroupID, &e.Description, &e.Amount, &e.Date, &e.SplitType, &e.Notes, &e.CreatedBy, &e.CreatedAt, &e.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		e.Payers = []ledger.Payer{}
		e.Shares = []ledger.Share{}
		idx[e.ID] = len(out)
		out = append(out, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}
	prow, err := q.QueryContext(ctx, `SELECT p.expense_id,p.user_id,p.amount FROM expense_payers p JOIN expenses e ON e.id=p.expense_id `+where+` ORDER BY p.rowid`, args...)
	if err != nil {
		return nil, err
	}
	for prow.Next() {
		var eid string
		var p ledger.Payer
		if err := prow.Scan(&eid, &p.UserID, &p.Amount); err != nil {
			prow.Close()
			return nil, err
		}
		if i, ok := idx[eid]; ok {
			out[i].Payers = append(out[i].Payers, p)
		}
	}
	prow.Close()
	srow, err := q.QueryContext(ctx, `SELECT s.expense_id,s.user_id,s.value,s.amount FROM expense_shares s JOIN expenses e ON e.id=s.expense_id `+where+` ORDER BY s.rowid`, args...)
	if err != nil {
		return nil, err
	}
	defer srow.Close()
	for srow.Next() {
		var eid string
		var s ledger.Share
		if err := srow.Scan(&eid, &s.UserID, &s.Value, &s.Amount); err != nil {
			return nil, err
		}
		if i, ok := idx[eid]; ok {
			out[i].Shares = append(out[i].Shares, s)
		}
	}
	return out, srow.Err()
}

// ---- payments ----

func InsertPayment(ctx context.Context, q Q, p ledger.Payment) error {
	_, err := q.ExecContext(ctx, `INSERT INTO payments(id,group_id,from_user_id,to_user_id,amount,date,notes,created_by,created_at) VALUES(?,?,?,?,?,?,?,?,?)`,
		p.ID, p.GroupID, p.FromUserID, p.ToUserID, p.Amount, p.Date, p.Notes, p.CreatedBy, p.CreatedAt)
	return err
}

func GetPayment(ctx context.Context, q Q, groupID, id string) (ledger.Payment, error) {
	var p ledger.Payment
	err := q.QueryRowContext(ctx, `SELECT id,group_id,from_user_id,to_user_id,amount,date,notes,created_by,created_at FROM payments WHERE group_id=? AND id=? AND deleted_at IS NULL`, groupID, id).
		Scan(&p.ID, &p.GroupID, &p.FromUserID, &p.ToUserID, &p.Amount, &p.Date, &p.Notes, &p.CreatedBy, &p.CreatedAt)
	if noRows(err) {
		return p, ErrNotFound
	}
	return p, err
}

func SoftDeletePayment(ctx context.Context, q Q, id string) error {
	_, err := q.ExecContext(ctx, `UPDATE payments SET deleted_at=? WHERE id=? AND deleted_at IS NULL`, db.Now(), id)
	return err
}

func Payments(ctx context.Context, q Q, groupID string) ([]ledger.Payment, error) {
	rows, err := q.QueryContext(ctx, `SELECT id,group_id,from_user_id,to_user_id,amount,date,notes,created_by,created_at FROM payments WHERE group_id=? AND deleted_at IS NULL ORDER BY date DESC, created_at DESC`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ledger.Payment{}
	for rows.Next() {
		var p ledger.Payment
		if err := rows.Scan(&p.ID, &p.GroupID, &p.FromUserID, &p.ToUserID, &p.Amount, &p.Date, &p.Notes, &p.CreatedBy, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ---- events ----

// AppendEvent bumps the group's sequence and inserts the event. Must run in
// the same transaction as the state change it describes.
func AppendEvent(ctx context.Context, q Q, groupID, typ string, actor ledger.User, payload any) (ledger.Event, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return ledger.Event{}, err
	}
	var seq int64
	if err := q.QueryRowContext(ctx, `UPDATE groups SET last_seq=last_seq+1 WHERE id=? RETURNING last_seq`, groupID).Scan(&seq); err != nil {
		return ledger.Event{}, err
	}
	ev := ledger.Event{GroupID: groupID, Seq: seq, Type: typ, ActorID: actor.ID, ActorName: actor.Name, Payload: payload, CreatedAt: db.Now()}
	if err := q.QueryRowContext(ctx, `INSERT INTO events(group_id,seq,type,actor_id,payload,created_at) VALUES(?,?,?,?,?,?) RETURNING id`,
		ev.GroupID, ev.Seq, ev.Type, ev.ActorID, string(raw), ev.CreatedAt).Scan(&ev.ID); err != nil {
		return ledger.Event{}, err
	}
	return ev, nil
}

func scanEvents(rows *sql.Rows) ([]ledger.Event, error) {
	defer rows.Close()
	out := []ledger.Event{}
	for rows.Next() {
		var ev ledger.Event
		var raw string
		if err := rows.Scan(&ev.ID, &ev.GroupID, &ev.Seq, &ev.Type, &ev.ActorID, &ev.ActorName, &raw, &ev.CreatedAt); err != nil {
			return nil, err
		}
		ev.Payload = json.RawMessage(raw)
		out = append(out, ev)
	}
	return out, rows.Err()
}

const eventCols = `e.id,e.group_id,e.seq,e.type,e.actor_id,u.name,e.payload,e.created_at FROM events e JOIN users u ON u.id=e.actor_id`

func GroupEvents(ctx context.Context, q Q, groupID string, since int64, limit int) ([]ledger.Event, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+eventCols+` WHERE e.group_id=? AND e.seq>? ORDER BY e.seq LIMIT ?`, groupID, since, limit)
	if err != nil {
		return nil, err
	}
	return scanEvents(rows)
}

// EventsAfter returns events with id > after across every group the user is in, ascending.
func EventsAfter(ctx context.Context, q Q, userID string, after int64, limit int) ([]ledger.Event, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+eventCols+` JOIN members m ON m.group_id=e.group_id AND m.user_id=? WHERE e.id>? ORDER BY e.id LIMIT ?`, userID, after, limit)
	if err != nil {
		return nil, err
	}
	return scanEvents(rows)
}

// Activity returns the newest events across the user's groups, newest first.
func Activity(ctx context.Context, q Q, userID string, limit int) ([]ledger.Event, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+eventCols+` JOIN members m ON m.group_id=e.group_id AND m.user_id=? ORDER BY e.id DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	return scanEvents(rows)
}

// ---- push ----

type PushSub struct {
	ID       string
	UserID   string
	Endpoint string
	P256dh   string
	Auth     string
}

func UpsertPushSub(ctx context.Context, q Q, s PushSub) error {
	_, err := q.ExecContext(ctx, `INSERT INTO push_subscriptions(id,user_id,endpoint,p256dh,auth,created_at) VALUES(?,?,?,?,?,?)
		ON CONFLICT(endpoint) DO UPDATE SET user_id=excluded.user_id, p256dh=excluded.p256dh, auth=excluded.auth`,
		db.NewID(), s.UserID, s.Endpoint, s.P256dh, s.Auth, db.Now())
	return err
}

func DeletePushSub(ctx context.Context, q Q, endpoint string) error {
	_, err := q.ExecContext(ctx, `DELETE FROM push_subscriptions WHERE endpoint=?`, endpoint)
	return err
}

func PushSubsForUsers(ctx context.Context, q Q, userIDs []string) ([]PushSub, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	args := make([]any, len(userIDs))
	ph := ""
	for i, id := range userIDs {
		args[i] = id
		if i > 0 {
			ph += ","
		}
		ph += "?"
	}
	rows, err := q.QueryContext(ctx, `SELECT id,user_id,endpoint,p256dh,auth FROM push_subscriptions WHERE user_id IN (`+ph+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PushSub
	for rows.Next() {
		var s PushSub
		if err := rows.Scan(&s.ID, &s.UserID, &s.Endpoint, &s.P256dh, &s.Auth); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
