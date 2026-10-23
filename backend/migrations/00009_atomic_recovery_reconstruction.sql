-- +goose Up
ALTER TABLE recovery_reconstructions
    ALTER CONSTRAINT recovery_reconstructions_sale_id_fkey
    DEFERRABLE INITIALLY DEFERRED;

-- +goose Down
ALTER TABLE recovery_reconstructions
    ALTER CONSTRAINT recovery_reconstructions_sale_id_fkey
    NOT DEFERRABLE;
