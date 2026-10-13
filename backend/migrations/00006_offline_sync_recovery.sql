-- +goose Up
CREATE TABLE sync_session_state (
    device_session_id UUID PRIMARY KEY REFERENCES pos_device_sessions(id),
    last_sequence BIGINT NOT NULL DEFAULT 0 CHECK (last_sequence >= 0),
    last_hash TEXT NOT NULL DEFAULT '' CHECK (last_hash = '' OR last_hash ~ '^[0-9a-f]{64}$'),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE sync_operations (
    operation_id UUID PRIMARY KEY,
    device_id UUID NOT NULL,
    device_session_id UUID NOT NULL REFERENCES pos_device_sessions(id),
    fencing_version BIGINT NOT NULL CHECK (fencing_version > 0),
    sequence BIGINT NOT NULL CHECK (sequence > 0),
    operation_type TEXT NOT NULL CHECK (operation_type IN ('sale')),
    previous_hash TEXT NOT NULL CHECK (previous_hash = '' OR previous_hash ~ '^[0-9a-f]{64}$'),
    payload_hash TEXT NOT NULL CHECK (payload_hash ~ '^[0-9a-f]{64}$'),
    payload JSONB NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('processing', 'acknowledged', 'recovery_evidence', 'conflict')),
    result_sale_id UUID REFERENCES sales(id),
    ack JSONB,
    created_at_local TIMESTAMPTZ NOT NULL,
    timezone TEXT NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    acknowledged_at TIMESTAMPTZ,
    UNIQUE (device_session_id, sequence)
);

CREATE INDEX sync_operations_pending_idx ON sync_operations(device_session_id, status, sequence);

CREATE TABLE device_recovery_cases (
    id UUID PRIMARY KEY,
    previous_session_id UUID NOT NULL UNIQUE REFERENCES pos_device_sessions(id),
    replacement_session_id UUID NOT NULL UNIQUE REFERENCES pos_device_sessions(id),
    affected_shift_id UUID REFERENCES cash_shifts(id),
    owner_id UUID NOT NULL REFERENCES users(id),
    status TEXT NOT NULL CHECK (status IN ('open', 'resolved')),
    reason TEXT NOT NULL CHECK (char_length(trim(reason)) BETWEEN 10 AND 500),
    last_ack_sequence BIGINT NOT NULL DEFAULT 0 CHECK (last_ack_sequence >= 0),
    last_ack_hash TEXT NOT NULL DEFAULT '' CHECK (last_ack_hash = '' OR last_ack_hash ~ '^[0-9a-f]{64}$'),
    cash_count_cents BIGINT NOT NULL CHECK (cash_count_cents >= 0),
    digital_payments_reviewed BOOLEAN NOT NULL DEFAULT FALSE,
    journal_status TEXT NOT NULL DEFAULT 'pending' CHECK (journal_status IN ('pending', 'imported', 'unrecoverable')),
    gaps_documented BOOLEAN NOT NULL DEFAULT FALSE,
    inventory_counted BOOLEAN NOT NULL DEFAULT FALSE,
    owner_approved BOOLEAN NOT NULL DEFAULT FALSE,
    opened_at TIMESTAMPTZ NOT NULL,
    resolved_at TIMESTAMPTZ
);

ALTER TABLE cash_shifts ADD COLUMN recovery_case_id UUID REFERENCES device_recovery_cases(id);

CREATE TABLE recovery_differences (
    id UUID PRIMARY KEY,
    recovery_case_id UUID NOT NULL REFERENCES device_recovery_cases(id),
    kind TEXT NOT NULL CHECK (kind IN ('unknown_sale_total', 'cash', 'digital_payment', 'inventory', 'sequence_gap')),
    amount_cents BIGINT,
    description TEXT NOT NULL CHECK (char_length(trim(description)) BETWEEN 3 AND 500),
    actor_id UUID NOT NULL REFERENCES users(id),
    request_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    resolved_at TIMESTAMPTZ
);

CREATE INDEX recovery_differences_case_idx ON recovery_differences(recovery_case_id, created_at);

-- +goose Down
DROP TABLE recovery_differences;
ALTER TABLE cash_shifts DROP COLUMN recovery_case_id;
DROP TABLE device_recovery_cases;
DROP TABLE sync_operations;
DROP TABLE sync_session_state;
