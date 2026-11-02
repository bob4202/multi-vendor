-- +goose Up

-- Orders start 'pending' and must be accepted by the shop before a deadline;
-- a background job cancels any that run past it. The deadline is stored (not
-- derived from created_at) so the timeout can change without affecting
-- orders already placed, and so the customer page can show a countdown.
ALTER TABLE orders
    ADD COLUMN accept_deadline TIMESTAMPTZ NOT NULL DEFAULT now() + interval '5 minutes',
    ADD COLUMN accepted_at     TIMESTAMPTZ,
    ADD COLUMN cancelled_at    TIMESTAMPTZ;

-- ExpireOverdueOrders: only pending orders are ever scanned.
CREATE INDEX orders_pending_deadline_idx ON orders (accept_deadline)
    WHERE status = 'pending' AND deleted = false;

-- +goose Down
DROP INDEX IF EXISTS orders_pending_deadline_idx;
ALTER TABLE orders
    DROP COLUMN cancelled_at,
    DROP COLUMN accepted_at,
    DROP COLUMN accept_deadline;
