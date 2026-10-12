-- +goose Up
CREATE TRIGGER sales_no_delete
BEFORE DELETE ON sales
FOR EACH ROW EXECUTE FUNCTION reject_pos_ledger_mutation();

CREATE TRIGGER sale_payments_no_delete
BEFORE DELETE ON sale_payments
FOR EACH ROW EXECUTE FUNCTION reject_pos_ledger_mutation();

-- +goose StatementBegin
CREATE FUNCTION protect_cash_shift_history()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'DELETE' OR OLD.status = 'closed' THEN
        RAISE EXCEPTION 'closed cash shifts are immutable'
            USING ERRCODE = '23514', CONSTRAINT = 'cash_shifts_closed_immutable';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER cash_shifts_history_guard
BEFORE UPDATE OR DELETE ON cash_shifts
FOR EACH ROW EXECUTE FUNCTION protect_cash_shift_history();

-- +goose Down
DROP TRIGGER IF EXISTS cash_shifts_history_guard ON cash_shifts;
DROP FUNCTION IF EXISTS protect_cash_shift_history();
DROP TRIGGER IF EXISTS sale_payments_no_delete ON sale_payments;
DROP TRIGGER IF EXISTS sales_no_delete ON sales;
