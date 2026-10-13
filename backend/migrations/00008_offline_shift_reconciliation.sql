-- +goose Up
ALTER TABLE sync_operations DROP CONSTRAINT sync_operations_operation_type_check;
ALTER TABLE sync_operations ADD CONSTRAINT sync_operations_operation_type_check
    CHECK (operation_type IN ('sale', 'shift_close', 'shift_open'));
ALTER TABLE sync_operations ADD COLUMN result_shift_id UUID REFERENCES cash_shifts(id);

CREATE TABLE recovery_reconstructions (
    id UUID PRIMARY KEY,
    recovery_case_id UUID NOT NULL REFERENCES device_recovery_cases(id),
    reconciliation_operation_id UUID NOT NULL UNIQUE,
    source_operation_id UUID UNIQUE,
    sale_id UUID NOT NULL UNIQUE REFERENCES sales(id),
    certainty TEXT NOT NULL CHECK (certainty IN ('exact', 'probable')),
    evidence TEXT NOT NULL CHECK (char_length(trim(evidence)) BETWEEN 3 AND 500),
    actor_id UUID NOT NULL REFERENCES users(id),
    request_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX recovery_reconstructions_case_idx
    ON recovery_reconstructions(recovery_case_id, created_at);

-- +goose Down
DROP TABLE recovery_reconstructions;
ALTER TABLE sync_operations DROP COLUMN result_shift_id;
ALTER TABLE sync_operations DROP CONSTRAINT sync_operations_operation_type_check;
ALTER TABLE sync_operations ADD CONSTRAINT sync_operations_operation_type_check
    CHECK (operation_type IN ('sale'));
