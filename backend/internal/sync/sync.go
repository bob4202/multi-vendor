// Package sync pulls Product and Customer records from each active branch's
// external inventory system (GET only) and upserts them into our database.
// The HTTP client lives in internal/inventory.
// Nothing here ever writes back to the inventory system — order management happens
// entirely on our side.
package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"ecommerce/backend/internal/crypto"
	"ecommerce/backend/internal/db"
	"ecommerce/backend/internal/inventory"
)

// resourceSyncer knows how to pull and persist one inventory resource.
type resourceSyncer interface {
	// Resource is the API path segment, e.g. "product".
	Resource() string
	// CursorParam is the query parameter used for incremental sync
	// (the inventory API names this differently per resource).
	CursorParam() string
	// PageLimit is the max page size this resource's docs allow.
	PageLimit() int
	// ExtraParams returns additional static query params to include on
	// every page request for this branch (e.g. a server-side filter
	// optimization). Returns nil when none apply.
	ExtraParams(branch db.Branch) url.Values
	// Upsert persists one raw record and returns its source last_modified
	// timestamp (used to advance the sync cursor even when the record
	// wasn't written) and whether a DB write actually happened.
	Upsert(ctx context.Context, q *db.Queries, branch db.Branch, raw json.RawMessage) (lastModified time.Time, written bool, err error)
}

// filterConfiguredChecker is implemented by resourceSyncers whose sync is
// gated by a per-branch inclusion filter (currently just products). When a
// branch has no filter configured, Engine skips calling the vendor API for
// that resource entirely rather than fetching everything and discarding it.
type filterConfiguredChecker interface {
	FilterConfigured(branch db.Branch) bool
}

type Engine struct {
	pool       *pgxpool.Pool
	queries    *db.Queries
	secretBox  *crypto.SecretBox
	apiURL     string // INVENTORY_API_URL, with a {host} placeholder
	httpClient *http.Client
	syncers    []resourceSyncer
	logger     *slog.Logger
}

func NewEngine(pool *pgxpool.Pool, secretBox *crypto.SecretBox, apiURL string, logger *slog.Logger) *Engine {
	return &Engine{
		pool:       pool,
		queries:    db.New(pool),
		secretBox:  secretBox,
		apiURL:     apiURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		syncers:    []resourceSyncer{productSyncer{}, customerSyncer{}},
		logger:     logger,
	}
}

// SyncBranch runs every resource syncer for one branch. A failure on one
// resource does not block the others.
func (e *Engine) SyncBranch(ctx context.Context, branch db.Branch) error {
	if e.apiURL == "" {
		return errors.New("INVENTORY_API_URL is not set")
	}
	apiKey, err := e.secretBox.Decrypt(branch.ApiKeyCipher)
	if err != nil {
		return fmt.Errorf("decrypt api key for branch %s: %w", branch.ID, err)
	}

	client := inventory.NewClient(e.apiURL, inventory.Credentials{
		Host:     branch.Host,
		Username: branch.ApiUsername,
		APIKey:   apiKey,
	}, e.httpClient)

	var firstErr error
	for _, syncer := range e.syncers {
		if err := e.syncResource(ctx, client, branch, syncer); err != nil {
			e.logger.Error("resource sync failed",
				"branch_id", branch.ID, "branch", branch.Name,
				"resource", syncer.Resource(), "error", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		e.logger.Info("resource sync succeeded",
			"branch_id", branch.ID, "branch", branch.Name, "resource", syncer.Resource())
	}
	return firstErr
}

func (e *Engine) syncResource(ctx context.Context, client *inventory.Client, branch db.Branch, syncer resourceSyncer) error {
	state, err := e.queries.GetSyncState(ctx, db.GetSyncStateParams{BranchID: branch.ID, Resource: syncer.Resource()})
	if err != nil && !isNoRows(err) {
		return fmt.Errorf("load sync state: %w", err)
	}

	if checker, ok := syncer.(filterConfiguredChecker); ok && !checker.FilterConfigured(branch) {
		e.logger.Warn("branch has no filter configured for resource; skipping sync entirely",
			"branch_id", branch.ID, "branch", branch.Name, "resource", syncer.Resource())
		zero := int32(0)
		return e.queries.RecordSyncSuccess(ctx, db.RecordSyncSuccessParams{
			BranchID:       branch.ID,
			Resource:       syncer.Resource(),
			Cursor:         state.Cursor,
			LastRunRecords: &zero,
		})
	}

	params := url.Values{}
	var cursor time.Time
	if state.Cursor.Valid {
		cursor = state.Cursor.Time
		params.Set(syncer.CursorParam(), cursor.UTC().Format(time.RFC3339))
	}
	for k, vs := range syncer.ExtraParams(branch) {
		for _, v := range vs {
			params.Add(k, v)
		}
	}

	maxSeen := cursor
	total := 0

	err = client.ForEachPage(ctx, syncer.Resource(), params, func(ctx context.Context, records []json.RawMessage) error {
		tx, err := e.pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx: %w", err)
		}
		defer tx.Rollback(ctx)

		q := e.queries.WithTx(tx)
		for _, rec := range records {
			lastModified, written, err := syncer.Upsert(ctx, q, branch, rec)
			if err != nil {
				return fmt.Errorf("upsert record: %w", err)
			}
			if lastModified.After(maxSeen) {
				maxSeen = lastModified
			}
			if written {
				total++
			}
		}
		return tx.Commit(ctx)
	})

	if err != nil {
		recErr := e.queries.RecordSyncFailure(ctx, db.RecordSyncFailureParams{
			BranchID:     branch.ID,
			Resource:     syncer.Resource(),
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
	records32 := int32(total)
	if err := e.queries.RecordSyncSuccess(ctx, db.RecordSyncSuccessParams{
		BranchID:       branch.ID,
		Resource:       syncer.Resource(),
		Cursor:         cursorParam,
		LastRunRecords: &records32,
	}); err != nil {
		return fmt.Errorf("record sync success: %w", err)
	}

	return nil
}

func strPtr(s string) *string { return &s }

func isNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
