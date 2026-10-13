-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION protect_sync_operation_envelope()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.operation_id <> OLD.operation_id
       OR NEW.device_id <> OLD.device_id
       OR NEW.device_session_id <> OLD.device_session_id
       OR NEW.fencing_version <> OLD.fencing_version
       OR NEW.sequence <> OLD.sequence
       OR NEW.operation_type <> OLD.operation_type
       OR NEW.previous_hash <> OLD.previous_hash
       OR NEW.payload_hash <> OLD.payload_hash
       OR NEW.payload IS DISTINCT FROM OLD.payload
       OR NEW.created_at_local <> OLD.created_at_local
       OR NEW.timezone <> OLD.timezone THEN
        RAISE EXCEPTION 'sync operation envelope is immutable'
            USING ERRCODE = '23514', CONSTRAINT = 'sync_operations_envelope_immutable';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER sync_operations_envelope_immutable
BEFORE UPDATE ON sync_operations
FOR EACH ROW EXECUTE FUNCTION protect_sync_operation_envelope();

-- +goose Down
DROP TRIGGER IF EXISTS sync_operations_envelope_immutable ON sync_operations;
DROP FUNCTION IF EXISTS protect_sync_operation_envelope();
