-- +goose Up
ALTER TABLE inventory_movements
    ADD COLUMN payload_hash TEXT;

ALTER TABLE inventory_movements
    ADD CONSTRAINT inventory_movements_payload_hash_check
    CHECK (payload_hash IS NULL OR payload_hash ~ '^[0-9a-f]{64}$');

-- +goose StatementBegin
CREATE FUNCTION apply_inventory_movement_to_balance()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    product_base_unit TEXT;
BEGIN
    SELECT base_unit
    INTO STRICT product_base_unit
    FROM products
    WHERE id = NEW.product_id;

    IF product_base_unit = 'unit' AND NEW.quantity_delta <> trunc(NEW.quantity_delta) THEN
        RAISE EXCEPTION 'unit products require integer inventory quantities'
            USING ERRCODE = '23514',
                  CONSTRAINT = 'inventory_movements_unit_quantity_check';
    END IF;

    PERFORM set_config('bakery.inventory_projection_write', 'on', TRUE);
    INSERT INTO inventory_balances (product_id, quantity, updated_at)
    VALUES (NEW.product_id, NEW.quantity_delta, NEW.created_at)
    ON CONFLICT (product_id) DO UPDATE
    SET quantity = inventory_balances.quantity + EXCLUDED.quantity,
        updated_at = EXCLUDED.updated_at;
    PERFORM set_config('bakery.inventory_projection_write', 'off', TRUE);

    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER inventory_movements_apply_balance
AFTER INSERT ON inventory_movements
FOR EACH ROW
EXECUTE FUNCTION apply_inventory_movement_to_balance();

INSERT INTO inventory_balances (product_id, quantity, updated_at)
SELECT p.id, COALESCE(sum(m.quantity_delta), 0), now()
FROM products p
LEFT JOIN inventory_movements m ON m.product_id = p.id
GROUP BY p.id
ON CONFLICT (product_id) DO UPDATE
SET quantity = EXCLUDED.quantity,
    updated_at = EXCLUDED.updated_at;

-- +goose StatementBegin
CREATE FUNCTION reject_direct_inventory_balance_mutation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'INSERT' AND NEW.quantity = 0 THEN
        RETURN NEW;
    END IF;

    IF current_setting('bakery.inventory_projection_write', TRUE) IS DISTINCT FROM 'on' THEN
        RAISE EXCEPTION 'inventory balances can only change through movements'
            USING ERRCODE = '23514',
                  CONSTRAINT = 'inventory_balances_projection_only';
    END IF;

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER inventory_balances_projection_only
BEFORE INSERT OR UPDATE OR DELETE ON inventory_balances
FOR EACH ROW
EXECUTE FUNCTION reject_direct_inventory_balance_mutation();

-- +goose StatementBegin
CREATE FUNCTION reject_inventory_movement_mutation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'inventory movements are immutable'
        USING ERRCODE = '23514',
              CONSTRAINT = 'inventory_movements_immutable';
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER inventory_movements_immutable
BEFORE UPDATE OR DELETE ON inventory_movements
FOR EACH ROW
EXECUTE FUNCTION reject_inventory_movement_mutation();

-- +goose Down
DROP TRIGGER IF EXISTS inventory_movements_immutable ON inventory_movements;
DROP FUNCTION IF EXISTS reject_inventory_movement_mutation();
DROP TRIGGER IF EXISTS inventory_balances_projection_only ON inventory_balances;
DROP FUNCTION IF EXISTS reject_direct_inventory_balance_mutation();
DROP TRIGGER IF EXISTS inventory_movements_apply_balance ON inventory_movements;
DROP FUNCTION IF EXISTS apply_inventory_movement_to_balance();
ALTER TABLE inventory_movements
    DROP CONSTRAINT IF EXISTS inventory_movements_payload_hash_check;
ALTER TABLE inventory_movements
    DROP COLUMN IF EXISTS payload_hash;
