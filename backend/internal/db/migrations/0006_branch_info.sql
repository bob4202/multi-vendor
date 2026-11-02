-- +goose Up

-- Storefront-facing branch details: contact, address, fulfilment options and
-- opening hours. Kept out of `branches` (which is inventory sync config and
-- credentials) so the two concerns can change independently - same split as
-- product_content vs products. Money values are in pence (BIGINT, matching
-- branch_products.price*).

-- Shared trigger function to keep updated_at fresh on UPDATE ------------------

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- branch_info: one row per branch (1:1 with branches) ---------------------------

CREATE TABLE branch_info (
    branch_id      UUID PRIMARY KEY REFERENCES branches(id) ON DELETE CASCADE,
    phone          TEXT NOT NULL,
    email          TEXT,
    address_line_1 TEXT,
    address_line_2 TEXT,
    city           TEXT,
    postal_code    TEXT,
    country        TEXT NOT NULL DEFAULT 'GB',
    timezone       TEXT NOT NULL DEFAULT 'Europe/London',

    collection                 BOOLEAN NOT NULL DEFAULT false,
    delivery                   BOOLEAN NOT NULL DEFAULT false,
    delivery_fee               BIGINT NOT NULL DEFAULT 0 CHECK (delivery_fee >= 0),
    delivery_free_over         BIGINT CHECK (delivery_free_over >= 0),           -- NULL = no free-delivery threshold
    delivery_min_order         BIGINT NOT NULL DEFAULT 0 CHECK (delivery_min_order >= 0),
    delivery_postcode_prefixes TEXT[] NOT NULL DEFAULT '{}',                     -- outward codes, uppercase, no spaces, e.g. {'LS1','LS2'}

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER trg_branch_info_updated_at
    BEFORE UPDATE ON branch_info
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- branch_opening_hours: weekly schedule, one row per open period ---------------
-- A day with no rows is closed. Two rows on one day = split hours (e.g. lunch).

CREATE TABLE branch_opening_hours (
    id          SERIAL PRIMARY KEY,
    branch_id   UUID NOT NULL REFERENCES branch_info(branch_id) ON DELETE CASCADE,
    day_of_week SMALLINT NOT NULL CHECK (day_of_week BETWEEN 1 AND 7), -- ISO: 1 = Monday ... 7 = Sunday
    open_time   TIME NOT NULL,
    close_time  TIME NOT NULL,
    CHECK (close_time > open_time),
    UNIQUE (branch_id, day_of_week, open_time)
);

-- branch_hours_exception: date-specific overrides (Eid, Christmas, etc.) -------
-- No rows for a date = use the weekly schedule.
-- closed = true  -> closed all day (times must be NULL).
-- closed = false -> special hours for that date (times required).

CREATE TABLE branch_hours_exception (
    id         SERIAL PRIMARY KEY,
    branch_id  UUID NOT NULL REFERENCES branch_info(branch_id) ON DELETE CASCADE,
    date       DATE NOT NULL,
    closed     BOOLEAN NOT NULL DEFAULT true,
    open_time  TIME,
    close_time TIME,
    note       TEXT,
    CHECK (
        (closed AND open_time IS NULL AND close_time IS NULL)
        OR
        (NOT closed AND open_time IS NOT NULL AND close_time IS NOT NULL AND close_time > open_time)
    ),
    UNIQUE (branch_id, date, open_time)
);

-- The UNIQUE above can't stop duplicate closed-all-day rows (open_time is
-- NULL there, and NULLs never conflict), so cover that case separately.
CREATE UNIQUE INDEX branch_hours_exception_closed_day_idx
    ON branch_hours_exception (branch_id, date)
    WHERE closed;

-- Useful queries ---------------------------------------------------------------
--
-- Does this postcode fall in a delivery area? (exact outward-code match,
-- so 'LS1' does not match 'LS10')
--
--   SELECT upper(left(replace(:postcode, ' ', ''),
--                     length(replace(:postcode, ' ', '')) - 3)) = ANY(delivery_postcode_prefixes)
--   FROM branch_info WHERE branch_id = :branch_id;
--
-- Is the branch open right now? (ignores exceptions; check those first)
--
--   SELECT EXISTS (
--     SELECT 1
--     FROM branch_info b
--     JOIN branch_opening_hours h ON h.branch_id = b.branch_id
--     WHERE b.branch_id = :branch_id
--       AND h.day_of_week = EXTRACT(ISODOW FROM now() AT TIME ZONE b.timezone)
--       AND (now() AT TIME ZONE b.timezone)::time >= h.open_time
--       AND (now() AT TIME ZONE b.timezone)::time <  h.close_time
--   );

-- +goose Down
DROP TABLE IF EXISTS branch_hours_exception;
DROP TABLE IF EXISTS branch_opening_hours;
DROP TABLE IF EXISTS branch_info;
DROP FUNCTION IF EXISTS set_updated_at();
