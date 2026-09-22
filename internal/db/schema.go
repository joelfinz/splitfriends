package db

import (
	"context"
	"fmt"
)

// migrations are applied in order; the index+1 is the schema version.
var migrations = []string{
	`
CREATE TABLE users (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  created_at  TEXT NOT NULL
);

CREATE TABLE credentials (
  id               TEXT PRIMARY KEY,   -- base64url of the raw credential id
  user_id          TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name             TEXT NOT NULL DEFAULT '',
  data             BLOB NOT NULL,      -- JSON-encoded webauthn.Credential
  created_at       TEXT NOT NULL,
  last_used_at     TEXT
);
CREATE INDEX credentials_user ON credentials(user_id);

CREATE TABLE sessions (
  id          TEXT PRIMARY KEY,
  user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at  TEXT NOT NULL,
  expires_at  TEXT NOT NULL
);
CREATE INDEX sessions_user ON sessions(user_id);

CREATE TABLE groups (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  currency    TEXT NOT NULL,
  created_by  TEXT NOT NULL REFERENCES users(id),
  created_at  TEXT NOT NULL,
  last_seq    INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE members (
  group_id    TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  joined_at   TEXT NOT NULL,
  PRIMARY KEY (group_id, user_id)
);
CREATE INDEX members_user ON members(user_id);

CREATE TABLE invites (
  token       TEXT PRIMARY KEY,
  group_id    TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  created_by  TEXT NOT NULL REFERENCES users(id),
  created_at  TEXT NOT NULL,
  expires_at  TEXT NOT NULL
);

CREATE TABLE expenses (
  id           TEXT PRIMARY KEY,
  group_id     TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  description  TEXT NOT NULL,
  amount       INTEGER NOT NULL,
  date         TEXT NOT NULL,
  split_type   TEXT NOT NULL,
  notes        TEXT NOT NULL DEFAULT '',
  created_by   TEXT NOT NULL REFERENCES users(id),
  created_at   TEXT NOT NULL,
  updated_at   TEXT NOT NULL,
  deleted_at   TEXT
);
CREATE INDEX expenses_group ON expenses(group_id, date);

CREATE TABLE expense_payers (
  expense_id  TEXT NOT NULL REFERENCES expenses(id) ON DELETE CASCADE,
  user_id     TEXT NOT NULL REFERENCES users(id),
  amount      INTEGER NOT NULL,
  PRIMARY KEY (expense_id, user_id)
);

CREATE TABLE expense_shares (
  expense_id  TEXT NOT NULL REFERENCES expenses(id) ON DELETE CASCADE,
  user_id     TEXT NOT NULL REFERENCES users(id),
  value       INTEGER NOT NULL,   -- the input: exact minor units / basis points / share count / 0
  amount      INTEGER NOT NULL,   -- the computed owed amount in minor units
  PRIMARY KEY (expense_id, user_id)
);

CREATE TABLE payments (
  id            TEXT PRIMARY KEY,
  group_id      TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  from_user_id  TEXT NOT NULL REFERENCES users(id),
  to_user_id    TEXT NOT NULL REFERENCES users(id),
  amount        INTEGER NOT NULL,
  date          TEXT NOT NULL,
  notes         TEXT NOT NULL DEFAULT '',
  created_by    TEXT NOT NULL REFERENCES users(id),
  created_at    TEXT NOT NULL,
  deleted_at    TEXT
);
CREATE INDEX payments_group ON payments(group_id, date);

CREATE TABLE events (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  group_id    TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  seq         INTEGER NOT NULL,
  type        TEXT NOT NULL,
  actor_id    TEXT NOT NULL REFERENCES users(id),
  payload     TEXT NOT NULL,
  created_at  TEXT NOT NULL,
  UNIQUE (group_id, seq)
);

CREATE TABLE push_subscriptions (
  id          TEXT PRIMARY KEY,
  user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  endpoint    TEXT NOT NULL UNIQUE,
  p256dh      TEXT NOT NULL,
  auth        TEXT NOT NULL,
  created_at  TEXT NOT NULL
);
CREATE INDEX push_user ON push_subscriptions(user_id);

CREATE TABLE settings (
  key    TEXT PRIMARY KEY,
  value  TEXT NOT NULL
);
`,
	// v2: categories, request IPs, audit log.
	`
ALTER TABLE expenses ADD COLUMN category TEXT NOT NULL DEFAULT 'other';
ALTER TABLE sessions ADD COLUMN ip TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN user_agent TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN last_seen_at TEXT NOT NULL DEFAULT '';
ALTER TABLE events ADD COLUMN ip TEXT NOT NULL DEFAULT '';
CREATE INDEX events_actor ON events(actor_id, id);
CREATE INDEX expenses_deleted ON expenses(group_id, deleted_at);
CREATE INDEX payments_deleted ON payments(group_id, deleted_at);

CREATE TABLE audit_log (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     TEXT,                 -- NULL for anonymous (failed admin login)
  kind        TEXT NOT NULL,        -- auth.register, auth.login, auth.logout, passkey.added, passkey.removed, admin.login, admin.login_failed, admin.session_revoked
  detail      TEXT NOT NULL DEFAULT '',
  ip          TEXT NOT NULL DEFAULT '',
  user_agent  TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL
);
CREATE INDEX audit_user ON audit_log(user_id, id);
CREATE INDEX audit_created ON audit_log(created_at);
`,
}

func (d *DB) migrate(ctx context.Context) error {
	if _, err := d.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`); err != nil {
		return err
	}
	var version int
	if err := d.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_version`).Scan(&version); err != nil {
		return err
	}
	for i := version; i < len(migrations); i++ {
		tx, err := d.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, migrations[i]); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM schema_version; INSERT INTO schema_version(version) VALUES (?)`, i+1); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// Setting reads a key from the settings table; ok=false when missing.
func (d *DB) Setting(ctx context.Context, key string) (string, bool, error) {
	var v string
	err := d.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return "", false, nil
		}
		return "", false, err
	}
	return v, true, nil
}

func (d *DB) SetSetting(ctx context.Context, key, value string) error {
	_, err := d.ExecContext(ctx, `INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}
