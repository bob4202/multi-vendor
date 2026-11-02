-- name: GetSyncState :one
SELECT * FROM sync_state WHERE branch_id = $1 AND resource = $2;

-- name: RecordSyncSuccess :exec
INSERT INTO sync_state (branch_id, resource, cursor, last_run_at, last_run_status, last_run_error, last_run_records)
VALUES ($1, $2, $3, now(), 'success', NULL, $4)
ON CONFLICT (branch_id, resource) DO UPDATE
SET cursor = COALESCE(EXCLUDED.cursor, sync_state.cursor),
    last_run_at = now(),
    last_run_status = 'success',
    last_run_error = NULL,
    last_run_records = EXCLUDED.last_run_records;

-- name: RecordSyncFailure :exec
INSERT INTO sync_state (branch_id, resource, last_run_at, last_run_status, last_run_error)
VALUES ($1, $2, now(), 'error', $3)
ON CONFLICT (branch_id, resource) DO UPDATE
SET last_run_at = now(),
    last_run_status = 'error',
    last_run_error = EXCLUDED.last_run_error;

-- name: ListSyncState :many
SELECT * FROM sync_state WHERE branch_id = $1;
