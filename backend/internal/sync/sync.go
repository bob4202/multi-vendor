// Package sync pulls Product and Customer records from each active branch's
// external inventory system and upserts them into our database. Each branch
// names its system in branches.inventory_provider; the provider (see
// internal/inventory) turns that system's data into our shape, so nothing
// here knows any one system's API. Nothing is ever written back.
package sync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"ecommerce/backend/internal/crypto"
	"ecommerce/backend/internal/db"
	"ecommerce/backend/internal/inventory"
)

const (
	resourceProduct  = "product"
	resourceCustomer = "customer"
)

type Engine struct {
	pool      *pgxpool.Pool
	queries   *db.Queries
	secretBox *crypto.SecretBox
	providers inventory.Registry
	logger    *slog.Logger
}

func NewEngine(pool *pgxpool.Pool, secretBox *crypto.SecretBox, providers inventory.Registry, logger *slog.Logger) *Engine {
	return &Engine{
		pool:      pool,
		queries:   db.New(pool),
		secretBox: secretBox,
		providers: providers,
		logger:    logger,
	}
}

// ProviderNames lists the inventory providers this server can sync from.
func (e *Engine) ProviderNames() []string { return e.providers.Names() }

// HasProvider reports whether a branch may use this provider here.
func (e *Engine) HasProvider(name string) bool { return e.providers.Has(name) }

// SyncBranch syncs products, then customers, for one branch. A failure on
// one resource doesn't block the other.
func (e *Engine) SyncBranch(ctx context.Context, branch db.Branch) error {
	apiKey, err := e.secretBox.Decrypt(branch.ApiKeyCipher)
	if err != nil {
		return fmt.Errorf("decrypt api key for branch %s: %w", branch.ID, err)
	}
	provider, err := e.providers.Build(branch.InventoryProvider, inventory.Credentials{
		Host:     branch.Host,
		Username: branch.ApiUsername,
		APIKey:   apiKey,
	})
	if err != nil {
		return fmt.Errorf("inventory provider for branch %s: %w", branch.ID, err)
	}

	var firstErr error
	for _, run := range []struct {
		resource string
		sync     func(context.Context, inventory.Provider, db.Branch) error
	}{
		{resourceProduct, e.syncProducts},
		{resourceCustomer, e.syncCustomers},
	} {
		if err := run.sync(ctx, provider, branch); err != nil {
			e.logger.Error("resource sync failed",
				"branch_id", branch.ID, "branch", branch.Name, "resource", run.resource, "error", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		e.logger.Info("resource sync succeeded",
			"branch_id", branch.ID, "branch", branch.Name, "resource", run.resource)
	}
	return firstErr
}

func (e *Engine) syncProducts(ctx context.Context, p inventory.Provider, branch db.Branch) error {
	// A branch with no filter wants no products: there's no unfiltered
	// "whole catalogue" mode, so don't even ask the system.
	if !productFilterConfigured(branch) {
		e.logger.Warn("branch has no product filter configured; skipping product sync",
			"branch_id", branch.ID, "branch", branch.Name)
		return e.recordSkipped(ctx, branch, resourceProduct)
	}

	return e.syncResource(ctx, branch, resourceProduct, func(ctx context.Context, since time.Time, save saveFunc) error {
		q := inventory.ProductQuery{Since: since, Department: departmentHint(branch)}
		return p.Products(ctx, q, func(ctx context.Context, products []inventory.Product) error {
			return save(ctx, func(ctx context.Context, q *db.Queries) (time.Time, int, error) {
				var maxSeen time.Time
				written := 0
				for _, pr := range products {
					lm, ok, err := upsertProduct(ctx, q, branch, pr)
					if err != nil {
						return time.Time{}, 0, err
					}
					if lm.After(maxSeen) {
						maxSeen = lm
					}
					if ok {
						written++
					}
				}
				return maxSeen, written, nil
			})
		})
	})
}

func (e *Engine) syncCustomers(ctx context.Context, p inventory.Provider, branch db.Branch) error {
	return e.syncResource(ctx, branch, resourceCustomer, func(ctx context.Context, since time.Time, save saveFunc) error {
		return p.Customers(ctx, since, func(ctx context.Context, customers []inventory.Customer) error {
			return save(ctx, func(ctx context.Context, q *db.Queries) (time.Time, int, error) {
				var maxSeen time.Time
				for _, c := range customers {
					if err := upsertCustomer(ctx, q, branch, c); err != nil {
						return time.Time{}, 0, err
					}
					if c.LastModified.After(maxSeen) {
						maxSeen = c.LastModified
					}
				}
				return maxSeen, len(customers), nil
			})
		})
	})
}

// pageWriter writes one page of records and returns the newest change time
// it saw and how many rows it wrote.
type pageWriter func(ctx context.Context, q *db.Queries) (maxSeen time.Time, written int, err error)

// saveFunc runs a pageWriter in its own transaction.
type saveFunc func(ctx context.Context, write pageWriter) error

// syncResource loads the resource's cursor, lets pull fetch everything
// changed since then (saving each page in its own transaction), and records
// the outcome. The cursor only moves forward after a fully successful run.
func (e *Engine) syncResource(ctx context.Context, branch db.Branch, resource string, pull func(ctx context.Context, since time.Time, save saveFunc) error) error {
	state, err := e.queries.GetSyncState(ctx, db.GetSyncStateParams{BranchID: branch.ID, Resource: resource})
	if err != nil && !isNoRows(err) {
		return fmt.Errorf("load sync state: %w", err)
	}

	var cursor time.Time
	if state.Cursor.Valid {
		cursor = state.Cursor.Time
	}
	maxSeen := cursor
	total := 0

	save := func(ctx context.Context, write pageWriter) error {
		tx, err := e.pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx: %w", err)
		}
		defer tx.Rollback(ctx)

		seen, written, err := write(ctx, e.queries.WithTx(tx))
		if err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		if seen.After(maxSeen) {
			maxSeen = seen
		}
		total += written
		return nil
	}

	if err := pull(ctx, cursor, save); err != nil {
		recErr := e.queries.RecordSyncFailure(ctx, db.RecordSyncFailureParams{
			BranchID:     branch.ID,
			Resource:     resource,
			LastRunError: strPtr(err.Error()),
		})
		if recErr != nil {
			e.logger.Error("failed to record sync failure", "error", recErr)
		}
		return err
	}

	var cursorParam pgtype.Timestamptz
	if !maxSeen.IsZero() {
		_ = cursorParam.Scan(maxSeen)
	}
	records := int32(total)
	if err := e.queries.RecordSyncSuccess(ctx, db.RecordSyncSuccessParams{
		BranchID:       branch.ID,
		Resource:       resource,
		Cursor:         cursorParam,
		LastRunRecords: &records,
	}); err != nil {
		return fmt.Errorf("record sync success: %w", err)
	}
	return nil
}

// recordSkipped marks a resource as run with nothing to do, keeping its cursor.
func (e *Engine) recordSkipped(ctx context.Context, branch db.Branch, resource string) error {
	state, err := e.queries.GetSyncState(ctx, db.GetSyncStateParams{BranchID: branch.ID, Resource: resource})
	if err != nil && !isNoRows(err) {
		return fmt.Errorf("load sync state: %w", err)
	}
	zero := int32(0)
	return e.queries.RecordSyncSuccess(ctx, db.RecordSyncSuccessParams{
		BranchID:       branch.ID,
		Resource:       resource,
		Cursor:         state.Cursor,
		LastRunRecords: &zero,
	})
}

func strPtr(s string) *string { return &s }

func isNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

// timestamptz turns a zero time into NULL.
func timestamptz(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// rawOrEmpty stores "{}" when a provider has no original record to keep.
func rawOrEmpty(raw []byte) []byte {
	if len(raw) == 0 {
		return []byte("{}")
	}
	return raw
}

func timeOrZero(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}
