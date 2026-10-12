-- +goose Up
CREATE TABLE pos_state (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    fencing_version BIGINT NOT NULL DEFAULT 0 CHECK (fencing_version >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO pos_state (singleton) VALUES (TRUE);

CREATE TABLE pos_device_sessions (
    id UUID PRIMARY KEY,
    device_id UUID NOT NULL,
    device_name TEXT NOT NULL CHECK (char_length(trim(device_name)) BETWEEN 1 AND 80),
    user_id UUID NOT NULL REFERENCES users(id),
    status TEXT NOT NULL CHECK (status IN ('active', 'released', 'recovery_required')),
    fencing_version BIGINT NOT NULL CHECK (fencing_version > 0),
    acquired_at TIMESTAMPTZ NOT NULL,
    released_at TIMESTAMPTZ,
    release_request_id TEXT,
    CHECK ((status = 'active' AND released_at IS NULL) OR status <> 'active')
);

CREATE UNIQUE INDEX pos_device_sessions_one_active_idx
    ON pos_device_sessions ((status)) WHERE status = 'active';
CREATE INDEX pos_device_sessions_device_idx
    ON pos_device_sessions (device_id, acquired_at DESC);

CREATE TABLE cash_shifts (
    id UUID PRIMARY KEY,
    status TEXT NOT NULL CHECK (status IN ('open', 'closed', 'provisional_closed', 'reconciliation_required')),
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    device_session_id UUID NOT NULL REFERENCES pos_device_sessions(id),
    opened_by UUID NOT NULL REFERENCES users(id),
    opening_cash_cents BIGINT NOT NULL CHECK (opening_cash_cents >= 0),
    opening_note TEXT,
    opened_at TIMESTAMPTZ NOT NULL,
    closed_by UUID REFERENCES users(id),
    counted_cash_cents BIGINT CHECK (counted_cash_cents >= 0),
    expected_cash_cents BIGINT,
    difference_cents BIGINT,
    closing_note TEXT,
    closed_at TIMESTAMPTZ,
    CHECK (
        (status = 'closed' AND closed_by IS NOT NULL AND counted_cash_cents IS NOT NULL
            AND expected_cash_cents IS NOT NULL AND difference_cents IS NOT NULL AND closed_at IS NOT NULL)
        OR status <> 'closed'
    )
);

CREATE UNIQUE INDEX cash_shifts_one_open_idx
    ON cash_shifts ((status)) WHERE status = 'open';
CREATE INDEX cash_shifts_opened_at_idx ON cash_shifts (opened_at DESC);

CREATE TABLE sales (
    id UUID PRIMARY KEY,
    cash_shift_id UUID NOT NULL REFERENCES cash_shifts(id),
    device_session_id UUID NOT NULL REFERENCES pos_device_sessions(id),
    fencing_version BIGINT NOT NULL CHECK (fencing_version > 0),
    actor_id UUID NOT NULL REFERENCES users(id),
    status TEXT NOT NULL CHECK (status IN ('completed', 'partially_returned', 'returned', 'voided')),
    payment_state TEXT NOT NULL CHECK (payment_state IN ('unverified', 'settled', 'issue')),
    catalog_version BIGINT NOT NULL CHECK (catalog_version > 0),
    subtotal_cents BIGINT NOT NULL CHECK (subtotal_cents > 0),
    total_cents BIGINT NOT NULL CHECK (total_cents > 0),
    request_id TEXT NOT NULL,
    payload_hash TEXT NOT NULL CHECK (payload_hash ~ '^[0-9a-f]{64}$'),
    completed_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX sales_completed_at_idx ON sales (completed_at DESC, id);
CREATE INDEX sales_shift_idx ON sales (cash_shift_id, completed_at DESC);

CREATE TABLE sale_items (
    id UUID PRIMARY KEY,
    sale_id UUID NOT NULL REFERENCES sales(id),
    line_number INTEGER NOT NULL CHECK (line_number > 0),
    product_id UUID NOT NULL REFERENCES products(id),
    product_name TEXT NOT NULL,
    sale_mode TEXT NOT NULL CHECK (sale_mode IN ('unit', 'weight')),
    base_unit TEXT NOT NULL CHECK (base_unit IN ('unit', 'g')),
    quantity NUMERIC(18,3) NOT NULL CHECK (quantity > 0),
    unit_price_cents BIGINT NOT NULL CHECK (unit_price_cents > 0),
    price_unit TEXT NOT NULL CHECK (price_unit IN ('unit', 'kg')),
    subtotal_cents BIGINT NOT NULL CHECK (subtotal_cents > 0),
    catalog_version BIGINT NOT NULL CHECK (catalog_version > 0),
    UNIQUE (sale_id, line_number),
    UNIQUE (sale_id, product_id)
);

CREATE TABLE sale_payments (
    id UUID PRIMARY KEY,
    sale_id UUID NOT NULL REFERENCES sales(id),
    method TEXT NOT NULL CHECK (method IN ('cash', 'transfer', 'qr', 'card', 'other')),
    amount_cents BIGINT NOT NULL CHECK (amount_cents > 0),
    status TEXT NOT NULL CHECK (status IN ('pending', 'reported', 'confirmed', 'rejected')),
    received_cents BIGINT,
    change_cents BIGINT,
    external_reference TEXT,
    confirmed_by UUID REFERENCES users(id),
    confirmed_at TIMESTAMPTZ,
    verification_method TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    CHECK (
        (method = 'cash' AND status = 'confirmed' AND received_cents IS NOT NULL
            AND change_cents IS NOT NULL AND received_cents >= amount_cents
            AND change_cents = received_cents - amount_cents)
        OR method <> 'cash'
    ),
    CHECK (
        (status = 'confirmed' AND confirmed_by IS NOT NULL AND confirmed_at IS NOT NULL
            AND verification_method IS NOT NULL)
        OR status <> 'confirmed'
    )
);

CREATE INDEX sale_payments_sale_idx ON sale_payments (sale_id, created_at, id);

CREATE TABLE cash_movements (
    id UUID PRIMARY KEY,
    cash_shift_id UUID NOT NULL REFERENCES cash_shifts(id),
    movement_type TEXT NOT NULL CHECK (movement_type IN ('sale', 'income', 'expense', 'withdrawal', 'refund')),
    amount_cents BIGINT NOT NULL CHECK (amount_cents > 0),
    reason TEXT,
    recipient TEXT,
    sale_id UUID REFERENCES sales(id),
    actor_id UUID NOT NULL REFERENCES users(id),
    request_id TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (movement_type = 'sale' OR char_length(trim(COALESCE(reason, ''))) BETWEEN 3 AND 300),
    CHECK (movement_type <> 'withdrawal' OR char_length(trim(COALESCE(recipient, ''))) BETWEEN 2 AND 120),
    CHECK ((movement_type = 'sale' AND sale_id IS NOT NULL) OR movement_type <> 'sale')
);

CREATE UNIQUE INDEX cash_movements_sale_unique_idx
    ON cash_movements (sale_id) WHERE movement_type = 'sale';
CREATE INDEX cash_movements_shift_idx
    ON cash_movements (cash_shift_id, occurred_at, id);

CREATE TABLE inventory_incidents (
    id UUID PRIMARY KEY,
    incident_type TEXT NOT NULL CHECK (incident_type IN ('negative_stock')),
    status TEXT NOT NULL CHECK (status IN ('open', 'resolved')),
    product_id UUID NOT NULL REFERENCES products(id),
    sale_id UUID NOT NULL REFERENCES sales(id),
    balance_quantity NUMERIC(18,3) NOT NULL CHECK (balance_quantity < 0),
    actor_id UUID NOT NULL REFERENCES users(id),
    request_id TEXT NOT NULL,
    opened_at TIMESTAMPTZ NOT NULL,
    resolved_at TIMESTAMPTZ,
    UNIQUE (sale_id, product_id)
);

-- +goose StatementBegin
CREATE FUNCTION reject_pos_ledger_mutation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION '% is immutable', TG_TABLE_NAME
        USING ERRCODE = '23514', CONSTRAINT = TG_TABLE_NAME || '_immutable';
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER sale_items_immutable
BEFORE UPDATE OR DELETE ON sale_items
FOR EACH ROW EXECUTE FUNCTION reject_pos_ledger_mutation();

CREATE TRIGGER cash_movements_immutable
BEFORE UPDATE OR DELETE ON cash_movements
FOR EACH ROW EXECUTE FUNCTION reject_pos_ledger_mutation();

-- +goose Down
DROP TRIGGER IF EXISTS cash_movements_immutable ON cash_movements;
DROP TRIGGER IF EXISTS sale_items_immutable ON sale_items;
DROP FUNCTION IF EXISTS reject_pos_ledger_mutation();
DROP TABLE inventory_incidents;
DROP TABLE cash_movements;
DROP TABLE sale_payments;
DROP TABLE sale_items;
DROP TABLE sales;
DROP TABLE cash_shifts;
DROP TABLE pos_device_sessions;
DROP TABLE pos_state;
