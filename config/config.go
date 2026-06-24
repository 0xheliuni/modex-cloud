// Package config loads and validates startup configuration from the environment.
//
// Security posture: the platform refuses to start without a strong master key and
// session secret. There are no insecure defaults for secrets — a missing or weak
// MASTER_KEK is a fatal error, not a warning.
package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config is the validated runtime configuration.
type Config struct {
	Port           int
	MasterKEKHex   string // 64-char hex (AES-256). Never logged.
	SessionSecret  string // >= 32 bytes recommended. Never logged.
	SQLDSN         string // empty => SQLite file
	RootUsername   string
	RootPassword   string // only used to seed the first admin
	TrustedProxies string
}

// Load reads configuration from the environment and validates it. It returns a
// descriptive error for any missing/weak required value so startup fails loudly.
//
// A .env file in the project root (if present) is loaded first, but never
// overrides variables already set in the real environment.
func Load() (*Config, error) {
	LoadDotEnv()

	c := &Config{
		Port:           envInt("PORT", 3000),
		MasterKEKHex:   os.Getenv("MASTER_KEK"),
		SessionSecret:  os.Getenv("SESSION_SECRET"),
		SQLDSN:         os.Getenv("SQL_DSN"),
		RootUsername:   envStr("ROOT_USERNAME", "admin"),
		RootPassword:   os.Getenv("ROOT_PASSWORD"),
		TrustedProxies: os.Getenv("TRUSTED_PROXIES"),
	}

	if len(c.MasterKEKHex) != 64 {
		return nil, fmt.Errorf("MASTER_KEK must be a 64-character hex string (AES-256); " +
			"generate one with: openssl rand -hex 32")
	}
	if !isHex(c.MasterKEKHex) {
		return nil, fmt.Errorf("MASTER_KEK must be valid hexadecimal")
	}
	if len(c.SessionSecret) < 16 {
		return nil, fmt.Errorf("SESSION_SECRET is required and must be at least 16 characters")
	}
	if c.RootPassword != "" && len(c.RootPassword) < 8 {
		return nil, fmt.Errorf("ROOT_PASSWORD, if set, must be at least 8 characters")
	}
	return c, nil
}

func isHex(s string) bool {
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
