package config

import (
	"os"
	"strings"
)

// Config is read once from the environment at startup.
type Config struct {
	Addr      string // listen address, e.g. ":8080"
	DataDir   string // directory holding splitfriends.db
	RPID      string // WebAuthn relying party ID, e.g. "split.example.com"
	Origin    string // public origin, e.g. "https://split.example.com"
	Origins   []string
	AppName   string
	PushEmail string // VAPID subscriber contact, "mailto:..."
	Dev       bool

	AdminPassword     string // empty disables /admin entirely
	TrustProxyHeaders bool   // read client IP from CF-Connecting-IP / X-Forwarded-For
}

func Load() Config {
	c := Config{
		Addr:      env("ADDR", ":8080"),
		DataDir:   env("DATA_DIR", "./data"),
		RPID:      env("RP_ID", "localhost"),
		Origin:    env("ORIGIN", "http://localhost:5173"),
		AppName:   env("APP_NAME", "Fairshare"),
		PushEmail: env("PUSH_CONTACT", "mailto:admin@example.com"),
		Dev:       env("DEV", "") != "",

		AdminPassword:     os.Getenv("ADMIN_PASSWORD"),
		TrustProxyHeaders: isTrue(env("TRUST_PROXY_HEADERS", "")),
	}
	// ORIGIN may be a comma separated list; the first is the canonical one used in invite URLs.
	for _, o := range strings.Split(c.Origin, ",") {
		if o = strings.TrimSpace(o); o != "" {
			c.Origins = append(c.Origins, o)
		}
	}
	c.Origin = c.Origins[0]
	return c
}

func isTrue(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
