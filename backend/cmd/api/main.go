// Command api runs the inventory sync service: an in-process scheduler that
// pulls Product and Customer records (GET only) from each active branch's
// inventory system, plus the HTTP API: platform admin, the vendor portal and
// the storefront (see internal/httpapi). This service never writes back to
// the inventory system.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"

	"ecommerce/backend/internal/config"
	"ecommerce/backend/internal/crypto"
	"ecommerce/backend/internal/customer"
	"ecommerce/backend/internal/db"
	"ecommerce/backend/internal/events"
	"ecommerce/backend/internal/httpapi"
	"ecommerce/backend/internal/inventory"
	inventoryconnect "ecommerce/backend/internal/inventory/connect"
	inventoryfake "ecommerce/backend/internal/inventory/fake"
	"ecommerce/backend/internal/mail"
	"ecommerce/backend/internal/notify"
	"ecommerce/backend/internal/payment"
	"ecommerce/backend/internal/payment/fake"
	"ecommerce/backend/internal/platform"
	"ecommerce/backend/internal/portal"
	"ecommerce/backend/internal/push"
	"ecommerce/backend/internal/scheduler"
	"ecommerce/backend/internal/store"
	"ecommerce/backend/internal/sync"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(logger); err != nil {
		logger.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if err := runMigrations(cfg.DatabaseURL); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	secretBox, err := crypto.NewSecretBox(cfg.EncryptionKey)
	if err != nil {
		return err
	}

	providers, err := buildPaymentRegistry(cfg.PaymentProviders)
	if err != nil {
		return err
	}
	logger.Info("payment providers", "enabled", cfg.PaymentProviders)

	payments := payment.NewService(pool, providers, secretBox, cfg.OrderAcceptTimeout, logger)

	queries := db.New(pool)
	inventoryProviders, err := buildInventoryRegistry(cfg.InventoryProviders, cfg.InventoryConnectURL)
	if err != nil {
		return err
	}
	logger.Info("inventory providers", "enabled", cfg.InventoryProviders)
	engine := sync.NewEngine(pool, secretBox, inventoryProviders, logger)
	sched := scheduler.New(engine, queries, cfg.SyncInterval, cfg.SyncConcurrency, logger)

	go sched.Run(ctx)

	orders := store.NewService(pool, store.Timeouts{
		Accept:       cfg.OrderAcceptTimeout,
		Substitution: cfg.SubstitutionTimeout,
		Collection:   cfg.CollectionWindow,
		Grace:        cfg.CollectionGrace,
		Payment:      cfg.PaymentWindow,
	}, logger)
	// Cancels orders the shop didn't accept in time. Every 30s is plenty:
	// accepting also checks the deadline itself, so a late click never wins.
	go orders.RunExpiry(ctx, 30*time.Second)

	// Live updates: one LISTEN connection fans order changes out to every
	// SSE stream this instance serves.
	hub := events.NewHub(logger)
	go hub.Run(ctx, pool)

	mailer := mail.New(mail.Config(cfg.SMTP), logger)
	pusher, err := push.New(cfg.VAPIDPublicKey, cfg.VAPIDPrivateKey, cfg.VAPIDSubject)
	if err != nil {
		return err
	}
	if pusher == nil {
		logger.Warn("VAPID keys not set - push notifications are off (run `make env`)")
	}

	// Order updates to customers (email + push), from notification_outbox.
	go notify.NewWorker(pool, mailer, pusher, hub.OutboxWake(), logger).Run(ctx)
	go payments.RunJobs(ctx)

	router := httpapi.NewRouter(httpapi.Deps{
		Queries:   queries,
		Engine:    engine,
		SecretBox: secretBox,
		Orders:    orders,
		Portal:    portal.NewService(pool).WithCollectionWindow(cfg.CollectionWindow),
		Platform:  platform.NewService(pool),
		Customers: customer.NewService(pool, mailer, logger),
		Events:    hub,
		Push:      pusher,
		AdminKey:  cfg.AdminAPIKey,
		Logger:    logger,
		Payments:  payments,
	})
	httpServer := &http.Server{Addr: cfg.HTTPAddr, Handler: router}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	logger.Info("http server listening", "addr", cfg.HTTPAddr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// knownProviders are the payment adapters compiled into this binary.
// PAYMENT_PROVIDERS picks which of them this server actually offers.
var knownProviders = payment.Registry{
	"fake": fake.New,
}

// buildPaymentRegistry keeps only the providers named in PAYMENT_PROVIDERS,
// and refuses to start on a name we have no adapter for (a typo like
// "strpie" should stop the deploy, not fail the first checkout).
func buildPaymentRegistry(names []string) (payment.Registry, error) {
	reg := payment.Registry{}
	for _, name := range names {
		factory, ok := knownProviders[name]
		if !ok {
			return nil, fmt.Errorf("PAYMENT_PROVIDERS: unknown provider %q", name)
		}
		reg[name] = factory
	}
	return reg, nil
}

// buildInventoryRegistry keeps only the inventory providers named in
// INVENTORY_PROVIDERS and refuses to start on an unknown name, or on connect
// without its URL, so a typo stops the deploy instead of the first sync.
func buildInventoryRegistry(names []string, connectURL string) (inventory.Registry, error) {
	known := inventory.Registry{
		inventoryconnect.Name: inventoryconnect.NewFactory(connectURL),
		inventoryfake.Name:    inventoryfake.New,
	}
	reg := inventory.Registry{}
	for _, name := range names {
		factory, ok := known[name]
		if !ok {
			return nil, fmt.Errorf("INVENTORY_PROVIDERS: unknown provider %q", name)
		}
		if name == inventoryconnect.Name && connectURL == "" {
			return nil, fmt.Errorf("INVENTORY_PROVIDERS: connect needs INVENTORY_CONNECT_URL")
		}
		reg[name] = factory
	}
	return reg, nil
}

func runMigrations(databaseURL string) error {
	sqlDB, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	return db.RunMigrations(sqlDB)
}
