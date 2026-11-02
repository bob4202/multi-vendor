// Package config loads process configuration from the environment.
package config

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL   string
	HTTPAddr      string
	AdminAPIKey   string
	EncryptionKey string // base64, 32 bytes decoded - used to encrypt branch API keys at rest
	// InventoryAPIURL is the inventory system's API base URL, with {host}
	// standing in for each branch's host. Sync is off while it's empty.
	InventoryAPIURL string
	SyncInterval    time.Duration
	SyncConcurrency int
	// OrderAcceptTimeout is how long a shop has to accept a new order before
	// it's cancelled automatically.
	OrderAcceptTimeout time.Duration

	// PaymentWindow is how long a customer has to finish paying for a new
	// order before it's cancelled automatically.
	PaymentWindow time.Duration

	// SubstitutionTimeout is how long a customer has to answer a proposed
	// substitution before the item is removed instead.
	SubstitutionTimeout time.Duration
	// CollectionWindow is how long a ready order waits to be collected;
	// CollectionGrace is the extra time before it becomes a no-show.
	CollectionWindow time.Duration
	CollectionGrace  time.Duration

	// PaymentProviders are the payment providers this server can use
	// (PAYMENT_PROVIDERS, comma-separated, e.g. "stripe"). Each vendor picks
	// one of them when connecting an account. Never list "fake" in production.
	PaymentProviders []string

	SMTP SMTPConfig
	// VAPID keys for Web Push (base64url, from `go run ./cmd/vapid`).
	// Empty = push notifications are off.
	VAPIDPublicKey  string
	VAPIDPrivateKey string
	VAPIDSubject    string
}

// SMTPConfig is where customer emails go out. Locally that's Mailpit
// (localhost:1025, no auth). Empty Host = emails are only logged.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		HTTPAddr:        getEnvDefault("HTTP_ADDR", ":8080"),
		AdminAPIKey:     os.Getenv("ADMIN_API_KEY"),
		EncryptionKey:   os.Getenv("APP_ENCRYPTION_KEY"),
		InventoryAPIURL: os.Getenv("INVENTORY_API_URL"),
		SyncInterval:    15 * time.Minute,
		SyncConcurrency: 3,

		OrderAcceptTimeout:  5 * time.Minute,
		SubstitutionTimeout: 10 * time.Minute,
		CollectionWindow:    60 * time.Minute,
		CollectionGrace:     6 * time.Minute,
		PaymentWindow:       30 * time.Minute,
		PaymentProviders:    parsePaymentProviders(os.Getenv("PAYMENT_PROVIDERS")),

		SMTP: SMTPConfig{
			Host:     os.Getenv("SMTP_HOST"),
			Port:     587,
			Username: os.Getenv("SMTP_USERNAME"),
			Password: os.Getenv("SMTP_PASSWORD"),
			From:     getEnvDefault("MAIL_FROM", "Grocery Shop <orders@groceryshop.example>"),
		},
		VAPIDPublicKey:  os.Getenv("VAPID_PUBLIC_KEY"),
		VAPIDPrivateKey: os.Getenv("VAPID_PRIVATE_KEY"),
		VAPIDSubject:    getEnvDefault("VAPID_SUBJECT", "mailto:orders@groceryshop.example"),
	}

	if cfg.DatabaseURL == "" {
		return cfg, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.AdminAPIKey == "" {
		return cfg, fmt.Errorf("ADMIN_API_KEY is required")
	}
	if cfg.EncryptionKey == "" {
		return cfg, fmt.Errorf("APP_ENCRYPTION_KEY is required (base64-encoded 32-byte key, e.g. `openssl rand -base64 32`)")
	}

	if v := os.Getenv("SYNC_INTERVAL_MINUTES"); v != "" {
		minutes, err := strconv.Atoi(v)
		if err != nil {
			return cfg, fmt.Errorf("invalid SYNC_INTERVAL_MINUTES: %w", err)
		}
		cfg.SyncInterval = time.Duration(minutes) * time.Minute
	}

	if v := os.Getenv("SYNC_BRANCH_CONCURRENCY"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return cfg, fmt.Errorf("invalid SYNC_BRANCH_CONCURRENCY: %w", err)
		}
		cfg.SyncConcurrency = n
	}

	if v := os.Getenv("ORDER_ACCEPT_TIMEOUT_MINUTES"); v != "" {
		minutes, err := strconv.Atoi(v)
		if err != nil || minutes < 1 {
			return cfg, fmt.Errorf("invalid ORDER_ACCEPT_TIMEOUT_MINUTES: must be a whole number of minutes, at least 1")
		}
		cfg.OrderAcceptTimeout = time.Duration(minutes) * time.Minute
	}

	for _, m := range []struct {
		env string
		dst *time.Duration
		min int
	}{
		{"SUBSTITUTION_TIMEOUT_MINUTES", &cfg.SubstitutionTimeout, 1},
		{"COLLECTION_WINDOW_MINUTES", &cfg.CollectionWindow, 1},
		{"COLLECTION_GRACE_MINUTES", &cfg.CollectionGrace, 0},
		{"PAYMENT_WINDOW_MINUTES", &cfg.PaymentWindow, 1},
	} {
		if v := os.Getenv(m.env); v != "" {
			minutes, err := strconv.Atoi(v)
			if err != nil || minutes < m.min {
				return cfg, fmt.Errorf("invalid %s: must be a whole number of minutes, at least %d", m.env, m.min)
			}
			*m.dst = time.Duration(minutes) * time.Minute
		}
	}

	if v := os.Getenv("SMTP_PORT"); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil || port < 1 || port > 65535 {
			return cfg, fmt.Errorf("invalid SMTP_PORT")
		}
		cfg.SMTP.Port = port
	}
	if len(cfg.PaymentProviders) == 0 {
		return cfg, fmt.Errorf("PAYMENT_PROVIDERS is required, e.g. PAYMENT_PROVIDERS=fake locally")
	}
	if (cfg.VAPIDPublicKey == "") != (cfg.VAPIDPrivateKey == "") {
		return cfg, fmt.Errorf("set both VAPID_PUBLIC_KEY and VAPID_PRIVATE_KEY, or neither")
	}

	return cfg, nil
}

// parsePaymentProviders turns "fake, Stripe," into ["fake", "stripe"]:
// trimmed, lowercased, without empties or duplicates. Whether a name is a
// provider we actually have is checked where the registry is built.
func parsePaymentProviders(s string) []string {
	var names []string
	for _, name := range strings.Split(s, ",") {
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "" && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}

func getEnvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
