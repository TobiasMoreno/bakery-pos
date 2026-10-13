package pos

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"bakery-pos/backend/internal/inventory"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool, now: time.Now}
}

func (r *PostgresRepository) AcquireDevice(ctx context.Context, input AcquireDeviceInput, actorID uuid.UUID, requestID string) (DeviceSession, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return DeviceSession{}, fmt.Errorf("begin acquire pos device: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var currentVersion int64
	if err := tx.QueryRow(ctx, `SELECT fencing_version FROM pos_state WHERE singleton = TRUE FOR UPDATE`).Scan(&currentVersion); err != nil {
		return DeviceSession{}, fmt.Errorf("lock pos state: %w", err)
	}
	current, err := activeDevice(ctx, tx)
	if err == nil {
		if current.DeviceID == input.DeviceID {
			return current, nil
		}
		return DeviceSession{}, fmt.Errorf("%w: another device is the active point of sale", ErrConflict)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return DeviceSession{}, fmt.Errorf("find active pos device: %w", err)
	}

	now := r.now().UTC()
	session := DeviceSession{
		ID: uuid.New(), DeviceID: input.DeviceID, DeviceName: input.DeviceName, UserID: actorID,
		Status: "active", FencingVersion: currentVersion + 1, AcquiredAt: now,
	}
	if _, err := tx.Exec(ctx, `UPDATE pos_state SET fencing_version = $1, updated_at = $2 WHERE singleton = TRUE`, session.FencingVersion, now); err != nil {
		return DeviceSession{}, fmt.Errorf("advance pos fencing version: %w", err)
	}
	const insert = `
		INSERT INTO pos_device_sessions (id, device_id, device_name, user_id, status, fencing_version, acquired_at)
		VALUES ($1, $2, $3, $4, 'active', $5, $6)`
	if _, err := tx.Exec(ctx, insert, session.ID, session.DeviceID, session.DeviceName, actorID, session.FencingVersion, now); err != nil {
		return DeviceSession{}, mapWriteError("insert pos device session", err)
	}
	if err := insertAudit(ctx, tx, actorID, "pos.device_acquired", "pos_device_session", session.ID, requestID, session); err != nil {
		return DeviceSession{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DeviceSession{}, mapWriteError("commit acquire pos device", err)
	}
	return session, nil
}

func (r *PostgresRepository) ReleaseDevice(ctx context.Context, input ReleaseDeviceInput, actorID uuid.UUID, requestID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin release pos device: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := activeDevice(ctx, tx)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNoActiveDevice
	}
	if err != nil {
		return fmt.Errorf("find active pos device: %w", err)
	}
	if current.ID != input.SessionID || current.FencingVersion != input.FencingVersion {
		return fmt.Errorf("%w: stale pos device session", ErrConflict)
	}
	var openShift bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM cash_shifts WHERE status = 'open')`).Scan(&openShift); err != nil {
		return fmt.Errorf("check open shift: %w", err)
	}
	if openShift {
		return fmt.Errorf("%w: close the cash shift before releasing the device", ErrConflict)
	}
	now := r.now().UTC()
	if _, err := tx.Exec(ctx, `UPDATE pos_device_sessions SET status = 'released', released_at = $2, release_request_id = $3 WHERE id = $1`, current.ID, now, requestID); err != nil {
		return mapWriteError("release pos device session", err)
	}
	if err := insertAudit(ctx, tx, actorID, "pos.device_released", "pos_device_session", current.ID, requestID, map[string]any{"fencing_version": current.FencingVersion}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit release pos device: %w", err)
	}
	return nil
}

func (r *PostgresRepository) OpenShift(ctx context.Context, input OpenShiftInput, actorID uuid.UUID, requestID string) (CashShift, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return CashShift{}, fmt.Errorf("begin open cash shift: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := requireDevice(ctx, tx, input.DeviceSessionID, input.FencingVersion); err != nil {
		return CashShift{}, err
	}
	now := r.now().UTC()
	shift := CashShift{
		ID: uuid.New(), Status: "open", Version: 1, DeviceSessionID: input.DeviceSessionID,
		OpenedBy: actorID, OpeningCashCents: input.OpeningCashCents, OpeningNote: input.OpeningNote,
		OpenedAt: now, ExpectedCashCents: input.OpeningCashCents,
	}
	const insert = `
		INSERT INTO cash_shifts (id, status, version, device_session_id, opened_by, opening_cash_cents, opening_note, opened_at)
		VALUES ($1, 'open', 1, $2, $3, $4, NULLIF($5, ''), $6)`
	if _, err := tx.Exec(ctx, insert, shift.ID, shift.DeviceSessionID, actorID, shift.OpeningCashCents, shift.OpeningNote, now); err != nil {
		return CashShift{}, mapWriteError("insert cash shift", err)
	}
	if err := insertAudit(ctx, tx, actorID, "cash.shift_opened", "cash_shift", shift.ID, requestID, shift); err != nil {
		return CashShift{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CashShift{}, mapWriteError("commit open cash shift", err)
	}
	return shift, nil
}

func (r *PostgresRepository) CurrentShift(ctx context.Context) (*CashShift, error) {
	shift, err := getShift(ctx, r.pool, uuid.Nil, false)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get current cash shift: %w", err)
	}
	return &shift, nil
}

func (r *PostgresRepository) RecordCashMovement(ctx context.Context, input RecordCashMovementInput, actorID uuid.UUID, requestID string) (CashShift, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return CashShift{}, fmt.Errorf("begin cash movement: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	shift, err := getShift(ctx, tx, input.CashShiftID, true)
	if errors.Is(err, pgx.ErrNoRows) {
		return CashShift{}, ErrNoOpenShift
	}
	if err != nil {
		return CashShift{}, fmt.Errorf("lock cash shift: %w", err)
	}
	if shift.Status != "open" || shift.Version != input.ExpectedVersion {
		return CashShift{}, fmt.Errorf("%w: cash shift version changed", ErrConflict)
	}
	now := r.now().UTC()
	const insert = `
		INSERT INTO cash_movements (id, cash_shift_id, movement_type, amount_cents, reason, recipient, actor_id, request_id, occurred_at)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7, $8, $9)`
	if _, err := tx.Exec(ctx, insert, input.ID, input.CashShiftID, input.MovementType, input.AmountCents, input.Reason, input.Recipient, actorID, requestID, now); err != nil {
		return CashShift{}, mapWriteError("insert cash movement", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE cash_shifts SET version = version + 1 WHERE id = $1`, shift.ID); err != nil {
		return CashShift{}, fmt.Errorf("advance cash shift version: %w", err)
	}
	if err := insertAudit(ctx, tx, actorID, "cash.movement_recorded", "cash_movement", input.ID, requestID, input); err != nil {
		return CashShift{}, err
	}
	updated, err := getShift(ctx, tx, shift.ID, false)
	if err != nil {
		return CashShift{}, fmt.Errorf("reload cash shift: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return CashShift{}, fmt.Errorf("commit cash movement: %w", err)
	}
	return updated, nil
}

func (r *PostgresRepository) CloseShift(ctx context.Context, input CloseShiftInput, actorID uuid.UUID, requestID string) (CashShift, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return CashShift{}, fmt.Errorf("begin close cash shift: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	shift, err := getShift(ctx, tx, input.CashShiftID, true)
	if errors.Is(err, pgx.ErrNoRows) {
		return CashShift{}, ErrNoOpenShift
	}
	if err != nil {
		return CashShift{}, fmt.Errorf("lock cash shift: %w", err)
	}
	if shift.Status != "open" || shift.Version != input.ExpectedVersion {
		return CashShift{}, fmt.Errorf("%w: cash shift version changed", ErrConflict)
	}
	now := r.now().UTC()
	difference := input.CountedCashCents - shift.ExpectedCashCents
	const update = `
		UPDATE cash_shifts
		SET status = 'closed', version = version + 1, closed_by = $2, counted_cash_cents = $3,
		    expected_cash_cents = $4, difference_cents = $5, closing_note = NULLIF($6, ''), closed_at = $7
		WHERE id = $1`
	if _, err := tx.Exec(ctx, update, shift.ID, actorID, input.CountedCashCents, shift.ExpectedCashCents, difference, input.ClosingNote, now); err != nil {
		return CashShift{}, mapWriteError("close cash shift", err)
	}
	if err := insertAudit(ctx, tx, actorID, "cash.shift_closed", "cash_shift", shift.ID, requestID, map[string]any{
		"previous_status": "open", "new_status": "closed", "counted_cash_cents": input.CountedCashCents,
		"expected_cash_cents": shift.ExpectedCashCents, "difference_cents": difference,
	}); err != nil {
		return CashShift{}, err
	}
	closed, err := getShift(ctx, tx, shift.ID, false)
	if err != nil {
		return CashShift{}, fmt.Errorf("reload closed cash shift: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return CashShift{}, fmt.Errorf("commit close cash shift: %w", err)
	}
	return closed, nil
}

func (r *PostgresRepository) CreateSale(ctx context.Context, input CreateSaleInput, actorID uuid.UUID, requestID, payloadHash string) (Sale, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return Sale{}, fmt.Errorf("begin sale: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var storedHash string
	err = tx.QueryRow(ctx, `SELECT payload_hash FROM sales WHERE id = $1`, input.OperationID).Scan(&storedHash)
	if err == nil {
		if storedHash != payloadHash {
			return Sale{}, fmt.Errorf("%w: operation id was already used with a different payload", ErrConflict)
		}
		sale, loadErr := getSale(ctx, tx, input.OperationID)
		if loadErr != nil {
			return Sale{}, loadErr
		}
		sale.Replayed = true
		return sale, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Sale{}, fmt.Errorf("find idempotent sale: %w", err)
	}
	if input.recoveryCaseID == nil {
		if err := requireDevice(ctx, tx, input.DeviceSessionID, input.FencingVersion); err != nil {
			return Sale{}, err
		}
	} else {
		var allowed bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM device_recovery_cases WHERE id=$1 AND status='open' AND previous_session_id=$2 AND affected_shift_id=$3)`, *input.recoveryCaseID, input.DeviceSessionID, input.CashShiftID).Scan(&allowed); err != nil {
			return Sale{}, fmt.Errorf("validate recovery reconstruction: %w", err)
		}
		if !allowed {
			return Sale{}, fmt.Errorf("%w: reconstruction does not belong to the recovery case", ErrConflict)
		}
	}
	shift, err := getShift(ctx, tx, input.CashShiftID, true)
	if errors.Is(err, pgx.ErrNoRows) {
		return Sale{}, ErrNoOpenShift
	}
	if err != nil {
		return Sale{}, fmt.Errorf("lock sale cash shift: %w", err)
	}
	validShiftStatus := shift.Status == "open" || (input.recoveryCaseID != nil && shift.Status == "reconciliation_required")
	if !validShiftStatus || shift.Version != input.ShiftVersion {
		return Sale{}, fmt.Errorf("%w: cash shift version changed", ErrConflict)
	}
	if shift.DeviceSessionID != input.DeviceSessionID {
		return Sale{}, fmt.Errorf("%w: cash shift belongs to another pos session", ErrConflict)
	}
	var currentCatalogVersion int64
	if err := tx.QueryRow(ctx, `SELECT version FROM catalog_state WHERE singleton = TRUE`).Scan(&currentCatalogVersion); err != nil {
		return Sale{}, fmt.Errorf("read catalog version: %w", err)
	}
	if input.offlineCatalogAt == nil && currentCatalogVersion != input.CatalogVersion {
		return Sale{}, fmt.Errorf("%w: catalog changed; refresh prices before charging", ErrConflict)
	}
	catalogVersion := input.CatalogVersion

	items := make([]SaleItem, 0, len(input.Items))
	var total int64
	for index, requested := range input.Items {
		item, err := loadPricedItem(ctx, tx, requested, catalogVersion, index+1, input.OperationID, input.offlineCatalogAt)
		if err != nil {
			return Sale{}, err
		}
		if item.SubtotalCents > math.MaxInt64-total {
			return Sale{}, fmt.Errorf("%w: sale total is out of range", ErrInvalid)
		}
		total += item.SubtotalCents
		items = append(items, item)
	}

	payments := make([]SalePayment, 0, len(input.Payments))
	var effectiveTotal, cashTotal int64
	paymentState := "settled"
	now := r.now().UTC()
	completedAt := now
	if input.offlineCreatedAt != nil {
		completedAt = input.offlineCreatedAt.UTC()
	}
	for index, requested := range input.Payments {
		if requested.AmountCents > math.MaxInt64-effectiveTotal {
			return Sale{}, fmt.Errorf("%w: payment total is out of range", ErrInvalid)
		}
		effectiveTotal += requested.AmountCents
		payment := SalePayment{
			ID:     uuid.NewSHA1(input.OperationID, []byte(fmt.Sprintf("payment:%d", index+1))),
			Method: requested.Method, AmountCents: requested.AmountCents, Status: requested.Status,
			ReceivedCents: requested.ReceivedCents, ExternalReference: strings.TrimSpace(requested.ExternalReference),
			VerificationMethod: strings.TrimSpace(requested.VerificationMethod),
		}
		if requested.Status == PaymentReported {
			paymentState = "unverified"
		}
		if requested.Status == PaymentConfirmed {
			payment.ConfirmedBy = &actorID
			payment.ConfirmedAt = &now
			if payment.Method == PaymentCash {
				payment.VerificationMethod = "cash_received"
			}
		}
		if payment.Method == PaymentCash {
			change := *payment.ReceivedCents - payment.AmountCents
			payment.ChangeCents = &change
			if payment.AmountCents > math.MaxInt64-cashTotal {
				return Sale{}, fmt.Errorf("%w: cash total is out of range", ErrInvalid)
			}
			cashTotal += payment.AmountCents
		}
		payments = append(payments, payment)
	}
	if effectiveTotal != total {
		return Sale{}, fmt.Errorf("%w: expected %d cents and received %d", ErrPaymentIncomplete, total, effectiveTotal)
	}

	sale := Sale{
		ID: input.OperationID, CashShiftID: input.CashShiftID, DeviceSessionID: input.DeviceSessionID,
		FencingVersion: input.FencingVersion, ActorID: actorID, Status: "completed", PaymentState: paymentState,
		CatalogVersion: catalogVersion, SubtotalCents: total, TotalCents: total, CompletedAt: completedAt,
		Items: items, Payments: payments, InventoryIncidents: make([]InventoryIncident, 0),
	}
	const insertSale = `
		INSERT INTO sales (id, cash_shift_id, device_session_id, fencing_version, actor_id, status,
		    payment_state, catalog_version, subtotal_cents, total_cents, request_id, payload_hash, completed_at, created_at)
		VALUES ($1, $2, $3, $4, $5, 'completed', $6, $7, $8, $8, $9, $10, $11, $11)`
	if _, err := tx.Exec(ctx, insertSale, sale.ID, sale.CashShiftID, sale.DeviceSessionID, sale.FencingVersion,
		actorID, sale.PaymentState, sale.CatalogVersion, sale.TotalCents, requestID, payloadHash, completedAt); err != nil {
		return Sale{}, mapWriteError("insert sale", err)
	}

	for _, item := range sale.Items {
		const insertItem = `
			INSERT INTO sale_items (id, sale_id, line_number, product_id, product_name, sale_mode, base_unit,
			    quantity, unit_price_cents, price_unit, subtotal_cents, catalog_version)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8::numeric, $9, $10, $11, $12)`
		if _, err := tx.Exec(ctx, insertItem, item.ID, sale.ID, item.LineNumber, item.ProductID, item.ProductName,
			item.SaleMode, item.BaseUnit, item.Quantity.String(), item.UnitPriceCents, item.PriceUnit, item.SubtotalCents, item.CatalogVersion); err != nil {
			return Sale{}, mapWriteError("insert sale item", err)
		}
		movementID := uuid.NewSHA1(sale.ID, []byte(fmt.Sprintf("inventory:%d", item.LineNumber)))
		movementHash := hashText(fmt.Sprintf("%s|%s|-%s", sale.ID, item.ProductID, item.Quantity.String()))
		const insertMovement = `
			INSERT INTO inventory_movements (id, product_id, quantity_delta, movement_type, reference_type,
			    reference_id, actor_id, request_id, reason, occurred_at, created_at, payload_hash)
			VALUES ($1, $2, $3::numeric, 'sale', 'sale', $4, $5, $6, NULL, $7, $7, $8)`
		if _, err := tx.Exec(ctx, insertMovement, movementID, item.ProductID, "-"+item.Quantity.String(), sale.ID, actorID, requestID, now, movementHash); err != nil {
			return Sale{}, mapWriteError("insert sale inventory movement", err)
		}
		var balanceText string
		if err := tx.QueryRow(ctx, `SELECT quantity::text FROM inventory_balances WHERE product_id = $1`, item.ProductID).Scan(&balanceText); err != nil {
			return Sale{}, fmt.Errorf("read post-sale inventory balance: %w", err)
		}
		balance, err := inventory.ParseQuantity(balanceText)
		if err != nil {
			return Sale{}, fmt.Errorf("parse post-sale inventory balance: %w", err)
		}
		if balance.Milliunits() < 0 {
			incident := InventoryIncident{
				ID:        uuid.NewSHA1(sale.ID, []byte("negative-stock:"+item.ProductID.String())),
				ProductID: item.ProductID, BalanceQuantity: balance, Status: "open",
			}
			const insertIncident = `
				INSERT INTO inventory_incidents (id, incident_type, status, product_id, sale_id, balance_quantity,
				    actor_id, request_id, opened_at)
				VALUES ($1, 'negative_stock', 'open', $2, $3, $4::numeric, $5, $6, $7)`
			if _, err := tx.Exec(ctx, insertIncident, incident.ID, incident.ProductID, sale.ID, balance.String(), actorID, requestID, now); err != nil {
				return Sale{}, mapWriteError("insert negative stock incident", err)
			}
			sale.InventoryIncidents = append(sale.InventoryIncidents, incident)
		}
	}

	for _, payment := range sale.Payments {
		const insertPayment = `
			INSERT INTO sale_payments (id, sale_id, method, amount_cents, status, received_cents, change_cents,
			    external_reference, confirmed_by, confirmed_at, verification_method, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), $9, $10, NULLIF($11, ''), $12)`
		if _, err := tx.Exec(ctx, insertPayment, payment.ID, sale.ID, payment.Method, payment.AmountCents, payment.Status,
			payment.ReceivedCents, payment.ChangeCents, payment.ExternalReference, payment.ConfirmedBy, payment.ConfirmedAt,
			payment.VerificationMethod, now); err != nil {
			return Sale{}, mapWriteError("insert sale payment", err)
		}
	}
	if cashTotal > 0 {
		cashMovementID := uuid.NewSHA1(sale.ID, []byte("cash"))
		const insertCash = `
			INSERT INTO cash_movements (id, cash_shift_id, movement_type, amount_cents, sale_id, actor_id, request_id, occurred_at)
			VALUES ($1, $2, 'sale', $3, $4, $5, $6, $7)`
		if _, err := tx.Exec(ctx, insertCash, cashMovementID, sale.CashShiftID, cashTotal, sale.ID, actorID, requestID, now); err != nil {
			return Sale{}, mapWriteError("insert sale cash movement", err)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE cash_shifts SET version = version + 1 WHERE id = $1`, sale.CashShiftID); err != nil {
		return Sale{}, fmt.Errorf("advance shift after sale: %w", err)
	}
	sale.CashShiftVersion = shift.Version + 1
	if err := insertAudit(ctx, tx, actorID, "sale.completed", "sale", sale.ID, requestID, map[string]any{
		"previous_status": "pending_payment", "new_status": "completed", "total_cents": sale.TotalCents,
		"payment_state": sale.PaymentState, "inventory_incidents": len(sale.InventoryIncidents),
	}); err != nil {
		return Sale{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Sale{}, mapWriteError("commit sale", err)
	}
	return sale, nil
}

func (r *PostgresRepository) ListSales(ctx context.Context, filter SaleFilter) ([]Sale, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM sales`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count sales: %w", err)
	}
	const query = `
		SELECT id
		FROM sales
		ORDER BY completed_at DESC, id DESC
		LIMIT $1 OFFSET $2`
	rows, err := r.pool.Query(ctx, query, filter.Limit, filter.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list sales: %w", err)
	}
	defer rows.Close()
	ids := make([]uuid.UUID, 0)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, 0, fmt.Errorf("scan sale id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate sales: %w", err)
	}
	sales := make([]Sale, 0, len(ids))
	for _, id := range ids {
		sale, err := getSale(ctx, r.pool, id)
		if err != nil {
			return nil, 0, err
		}
		sales = append(sales, sale)
	}
	return sales, total, nil
}

func (r *PostgresRepository) GetSale(ctx context.Context, id uuid.UUID) (Sale, error) {
	return getSale(ctx, r.pool, id)
}

type rowScanner interface {
	Scan(dest ...any) error
}

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func activeDevice(ctx context.Context, queryer queryer) (DeviceSession, error) {
	var session DeviceSession
	err := queryer.QueryRow(ctx, `
		SELECT id, device_id, device_name, user_id, status, fencing_version, acquired_at, released_at
		FROM pos_device_sessions
		WHERE status = 'active'
		FOR UPDATE`).Scan(&session.ID, &session.DeviceID, &session.DeviceName, &session.UserID, &session.Status,
		&session.FencingVersion, &session.AcquiredAt, &session.ReleasedAt)
	return session, err
}

func requireDevice(ctx context.Context, queryer queryer, sessionID uuid.UUID, fencingVersion int64) error {
	var status string
	var storedVersion int64
	err := queryer.QueryRow(ctx, `SELECT status, fencing_version FROM pos_device_sessions WHERE id = $1 FOR UPDATE`, sessionID).Scan(&status, &storedVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNoActiveDevice
	}
	if err != nil {
		return fmt.Errorf("validate pos device: %w", err)
	}
	if status != "active" || storedVersion != fencingVersion {
		return fmt.Errorf("%w: pos device session is no longer active", ErrNoActiveDevice)
	}
	return nil
}

func getShift(ctx context.Context, queryer queryer, id uuid.UUID, lock bool) (CashShift, error) {
	where := "WHERE s.status = 'open'"
	if id != uuid.Nil {
		where = "WHERE s.id = $1"
	}
	query := `
		SELECT s.id, s.status, s.version, s.device_session_id, s.opened_by, s.opening_cash_cents,
		       COALESCE(s.opening_note, ''), s.opened_at, s.closed_by, s.counted_cash_cents,
		       CASE WHEN s.status = 'closed' THEN s.expected_cash_cents
		            ELSE s.opening_cash_cents + COALESCE((
		                SELECT sum(CASE WHEN m.movement_type IN ('sale', 'income') THEN m.amount_cents ELSE -m.amount_cents END)
		                FROM cash_movements m WHERE m.cash_shift_id = s.id
		            ), 0) END,
		       s.difference_cents, COALESCE(s.closing_note, ''), s.closed_at
		FROM cash_shifts s ` + where
	if lock {
		query += ` FOR UPDATE OF s`
	}
	var row pgx.Row
	if id == uuid.Nil {
		row = queryer.QueryRow(ctx, query)
	} else {
		row = queryer.QueryRow(ctx, query, id)
	}
	var shift CashShift
	var closedBy uuid.NullUUID
	var counted, expected, difference sql.NullInt64
	if err := row.Scan(&shift.ID, &shift.Status, &shift.Version, &shift.DeviceSessionID, &shift.OpenedBy,
		&shift.OpeningCashCents, &shift.OpeningNote, &shift.OpenedAt, &closedBy, &counted, &expected,
		&difference, &shift.ClosingNote, &shift.ClosedAt); err != nil {
		return CashShift{}, err
	}
	if closedBy.Valid {
		shift.ClosedBy = &closedBy.UUID
	}
	if counted.Valid {
		shift.CountedCashCents = &counted.Int64
	}
	if expected.Valid {
		shift.ExpectedCashCents = expected.Int64
	}
	if difference.Valid {
		shift.DifferenceCents = &difference.Int64
	}
	return shift, nil
}

func loadPricedItem(ctx context.Context, queryer queryer, requested SaleItemInput, catalogVersion int64, lineNumber int, saleID uuid.UUID, priceAt *time.Time) (SaleItem, error) {
	var item SaleItem
	item.ID = uuid.NewSHA1(saleID, []byte(fmt.Sprintf("item:%d", lineNumber)))
	item.LineNumber = lineNumber
	item.ProductID = requested.ProductID
	item.Quantity = requested.Quantity
	item.CatalogVersion = catalogVersion
	const currentQuery = `
		SELECT p.name, p.sale_mode, p.base_unit, pp.amount_cents, pp.price_unit
		FROM products p
		JOIN product_prices pp ON pp.product_id = p.id AND pp.effective_to IS NULL
		WHERE p.id = $1 AND p.active = TRUE AND p.sellable = TRUE
		FOR SHARE OF p, pp`
	const historicalQuery = `
		SELECT p.name, p.sale_mode, p.base_unit, pp.amount_cents, pp.price_unit
		FROM products p
		JOIN product_prices pp ON pp.product_id = p.id
		WHERE p.id = $1 AND pp.effective_from <= $2
		  AND (pp.effective_to IS NULL OR pp.effective_to > $2)
		ORDER BY pp.effective_from DESC LIMIT 1
		FOR SHARE OF p, pp`
	var err error
	if priceAt == nil {
		err = queryer.QueryRow(ctx, currentQuery, requested.ProductID).Scan(&item.ProductName, &item.SaleMode, &item.BaseUnit, &item.UnitPriceCents, &item.PriceUnit)
	} else {
		err = queryer.QueryRow(ctx, historicalQuery, requested.ProductID, priceAt.UTC()).Scan(&item.ProductName, &item.SaleMode, &item.BaseUnit, &item.UnitPriceCents, &item.PriceUnit)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return SaleItem{}, fmt.Errorf("%w: product is inactive, not sellable or has no price", ErrNotFound)
	} else if err != nil {
		return SaleItem{}, fmt.Errorf("load sale product: %w", err)
	}
	subtotal, err := CalculateSubtotal(item.UnitPriceCents, item.SaleMode, item.Quantity)
	if err != nil {
		return SaleItem{}, err
	}
	item.SubtotalCents = subtotal
	return item, nil
}

func getSale(ctx context.Context, queryer queryer, id uuid.UUID) (Sale, error) {
	var sale Sale
	const header = `
		SELECT s.id, s.cash_shift_id, s.device_session_id, s.fencing_version, s.actor_id, s.status,
		       s.payment_state, s.catalog_version, s.subtotal_cents, s.total_cents, s.completed_at, cs.version
		FROM sales s
		JOIN cash_shifts cs ON cs.id = s.cash_shift_id
		WHERE s.id = $1`
	if err := queryer.QueryRow(ctx, header, id).Scan(&sale.ID, &sale.CashShiftID, &sale.DeviceSessionID, &sale.FencingVersion,
		&sale.ActorID, &sale.Status, &sale.PaymentState, &sale.CatalogVersion, &sale.SubtotalCents,
		&sale.TotalCents, &sale.CompletedAt, &sale.CashShiftVersion); errors.Is(err, pgx.ErrNoRows) {
		return Sale{}, ErrNotFound
	} else if err != nil {
		return Sale{}, fmt.Errorf("get sale: %w", err)
	}

	itemRows, err := queryer.Query(ctx, `
		SELECT id, line_number, product_id, product_name, sale_mode, base_unit, quantity::text,
		       unit_price_cents, price_unit, subtotal_cents, catalog_version
		FROM sale_items WHERE sale_id = $1 ORDER BY line_number`, id)
	if err != nil {
		return Sale{}, fmt.Errorf("list sale items: %w", err)
	}
	sale.Items = make([]SaleItem, 0)
	for itemRows.Next() {
		var item SaleItem
		var quantityText string
		if err := itemRows.Scan(&item.ID, &item.LineNumber, &item.ProductID, &item.ProductName, &item.SaleMode,
			&item.BaseUnit, &quantityText, &item.UnitPriceCents, &item.PriceUnit, &item.SubtotalCents, &item.CatalogVersion); err != nil {
			itemRows.Close()
			return Sale{}, fmt.Errorf("scan sale item: %w", err)
		}
		item.Quantity, err = inventory.ParseQuantity(quantityText)
		if err != nil {
			itemRows.Close()
			return Sale{}, fmt.Errorf("parse sale item quantity: %w", err)
		}
		sale.Items = append(sale.Items, item)
	}
	if err := itemRows.Err(); err != nil {
		itemRows.Close()
		return Sale{}, fmt.Errorf("iterate sale items: %w", err)
	}
	itemRows.Close()

	paymentRows, err := queryer.Query(ctx, `
		SELECT id, method, amount_cents, status, received_cents, change_cents,
		       COALESCE(external_reference, ''), confirmed_by, confirmed_at, COALESCE(verification_method, '')
		FROM sale_payments WHERE sale_id = $1 ORDER BY created_at, id`, id)
	if err != nil {
		return Sale{}, fmt.Errorf("list sale payments: %w", err)
	}
	sale.Payments = make([]SalePayment, 0)
	for paymentRows.Next() {
		var payment SalePayment
		if err := paymentRows.Scan(&payment.ID, &payment.Method, &payment.AmountCents, &payment.Status,
			&payment.ReceivedCents, &payment.ChangeCents, &payment.ExternalReference, &payment.ConfirmedBy,
			&payment.ConfirmedAt, &payment.VerificationMethod); err != nil {
			paymentRows.Close()
			return Sale{}, fmt.Errorf("scan sale payment: %w", err)
		}
		sale.Payments = append(sale.Payments, payment)
	}
	if err := paymentRows.Err(); err != nil {
		paymentRows.Close()
		return Sale{}, fmt.Errorf("iterate sale payments: %w", err)
	}
	paymentRows.Close()

	incidentRows, err := queryer.Query(ctx, `
		SELECT id, product_id, balance_quantity::text, status
		FROM inventory_incidents WHERE sale_id = $1 ORDER BY opened_at, id`, id)
	if err != nil {
		return Sale{}, fmt.Errorf("list sale inventory incidents: %w", err)
	}
	sale.InventoryIncidents = make([]InventoryIncident, 0)
	for incidentRows.Next() {
		var incident InventoryIncident
		var balanceText string
		if err := incidentRows.Scan(&incident.ID, &incident.ProductID, &balanceText, &incident.Status); err != nil {
			incidentRows.Close()
			return Sale{}, fmt.Errorf("scan inventory incident: %w", err)
		}
		incident.BalanceQuantity, err = inventory.ParseQuantity(balanceText)
		if err != nil {
			incidentRows.Close()
			return Sale{}, fmt.Errorf("parse incident balance: %w", err)
		}
		sale.InventoryIncidents = append(sale.InventoryIncidents, incident)
	}
	if err := incidentRows.Err(); err != nil {
		incidentRows.Close()
		return Sale{}, fmt.Errorf("iterate inventory incidents: %w", err)
	}
	incidentRows.Close()
	return sale, nil
}

func insertAudit(ctx context.Context, tx pgx.Tx, actorID uuid.UUID, action, entityType string, entityID uuid.UUID, requestID string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("encode pos audit data: %w", err)
	}
	const query = `
		INSERT INTO audit_events (id, actor_id, action, entity_type, entity_id, request_id, data)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)`
	if _, err := tx.Exec(ctx, query, uuid.New(), actorID, action, entityType, entityID, requestID, string(payload)); err != nil {
		return fmt.Errorf("insert pos audit event: %w", err)
	}
	return nil
}

func hashText(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func mapWriteError(operation string, err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505", "40001":
			return fmt.Errorf("%w: concurrent or duplicate operation", ErrConflict)
		case "23503":
			return fmt.Errorf("%w: referenced resource does not exist", ErrNotFound)
		case "23514", "22003":
			return fmt.Errorf("%w: pos constraint failed", ErrInvalid)
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}
