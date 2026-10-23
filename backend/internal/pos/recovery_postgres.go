package pos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *PostgresRepository) OpenOfflineShift(ctx context.Context, input OfflineOpenShiftInput, actorID uuid.UUID, requestID string) (CashShift, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return CashShift{}, fmt.Errorf("begin offline shift opening: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	existing, existingErr := getShift(ctx, tx, input.ID, false)
	if existingErr == nil {
		if existing.Status != "open" || existing.DeviceSessionID != input.DeviceSessionID || existing.OpeningCashCents != input.OpeningCashCents {
			return CashShift{}, fmt.Errorf("%w: offline shift id was reused with different content", ErrConflict)
		}
		return existing, nil
	}
	if !errors.Is(existingErr, pgx.ErrNoRows) {
		return CashShift{}, existingErr
	}
	if err := requireDevice(ctx, tx, input.DeviceSessionID, input.FencingVersion); err != nil {
		return CashShift{}, err
	}
	var previousStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM cash_shifts WHERE id = $1 FOR UPDATE`, input.PreviousCashShiftID).Scan(&previousStatus); err != nil {
		return CashShift{}, ErrNoOpenShift
	}
	if previousStatus != "closed" {
		return CashShift{}, fmt.Errorf("%w: previous offline shift must synchronize first", ErrConflict)
	}
	now := r.now().UTC()
	shift := CashShift{ID: input.ID, Status: "open", Version: 1, DeviceSessionID: input.DeviceSessionID, OpenedBy: actorID, OpeningCashCents: input.OpeningCashCents, OpeningNote: input.OpeningNote, OpenedAt: now, ExpectedCashCents: input.OpeningCashCents}
	_, err = tx.Exec(ctx, `INSERT INTO cash_shifts (id,status,version,device_session_id,opened_by,opening_cash_cents,opening_note,opened_at) VALUES ($1,'open',1,$2,$3,$4,NULLIF($5,''),$6)`, shift.ID, shift.DeviceSessionID, actorID, shift.OpeningCashCents, shift.OpeningNote, now)
	if err != nil {
		return CashShift{}, mapWriteError("insert offline cash shift", err)
	}
	if err := insertAudit(ctx, tx, actorID, "cash.shift_opened_offline", "cash_shift", shift.ID, requestID, map[string]any{"previous_shift_id": input.PreviousCashShiftID}); err != nil {
		return CashShift{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CashShift{}, mapWriteError("commit offline shift opening", err)
	}
	return shift, nil
}

func (r *PostgresRepository) CloseOfflineShift(ctx context.Context, input CloseShiftInput, actorID uuid.UUID, requestID string) (CashShift, error) {
	shift, err := getShift(ctx, r.pool, input.CashShiftID, false)
	if err != nil {
		return CashShift{}, err
	}
	if shift.Status == "closed" {
		if shift.Version != input.ExpectedVersion+1 || shift.CountedCashCents == nil || *shift.CountedCashCents != input.CountedCashCents || shift.ClosingNote != input.ClosingNote {
			return CashShift{}, fmt.Errorf("%w: offline close was already applied with different content", ErrConflict)
		}
		return shift, nil
	}
	return r.CloseShift(ctx, input, actorID, requestID)
}

func (r *PostgresRepository) GetActiveRecovery(ctx context.Context) (*RecoveryDetails, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `SELECT id FROM device_recovery_cases WHERE status = 'open' ORDER BY opened_at DESC LIMIT 1`).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find active recovery: %w", err)
	}
	details, err := getRecoveryDetails(ctx, r.pool, id)
	return &details, err
}

func (r *PostgresRepository) AddRecoveryDifference(ctx context.Context, caseID uuid.UUID, input RecoveryDifferenceInput, actorID uuid.UUID, requestID string) (RecoveryDetails, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return RecoveryDetails{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM device_recovery_cases WHERE id=$1 FOR UPDATE`, caseID).Scan(&status); errors.Is(err, pgx.ErrNoRows) {
		return RecoveryDetails{}, ErrNotFound
	} else if err != nil {
		return RecoveryDetails{}, fmt.Errorf("lock recovery case: %w", err)
	} else if status != "open" {
		return RecoveryDetails{}, fmt.Errorf("%w: recovery case is already resolved", ErrConflict)
	}
	now := r.now().UTC()
	command, err := tx.Exec(ctx, `INSERT INTO recovery_differences (id,recovery_case_id,kind,amount_cents,description,actor_id,request_id,created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT (id) DO NOTHING`, input.ID, caseID, input.Kind, input.AmountCents, input.Description, actorID, requestID, now)
	if err != nil {
		return RecoveryDetails{}, mapWriteError("insert recovery difference", err)
	}
	if command.RowsAffected() == 0 {
		var samePayload bool
		if err := tx.QueryRow(ctx, `SELECT recovery_case_id=$2 AND kind=$3 AND amount_cents IS NOT DISTINCT FROM $4 AND description=$5 FROM recovery_differences WHERE id=$1`, input.ID, caseID, input.Kind, input.AmountCents, input.Description).Scan(&samePayload); errors.Is(err, pgx.ErrNoRows) {
			return RecoveryDetails{}, ErrNotFound
		} else if err != nil {
			return RecoveryDetails{}, fmt.Errorf("read recovery difference replay: %w", err)
		} else if !samePayload {
			return RecoveryDetails{}, fmt.Errorf("%w: recovery difference id was reused with different content", ErrConflict)
		}
	} else if err := insertAudit(ctx, tx, actorID, "pos.recovery_difference_added", "device_recovery_case", caseID, requestID, map[string]any{"difference_id": input.ID, "kind": input.Kind}); err != nil {
		return RecoveryDetails{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RecoveryDetails{}, err
	}
	return getRecoveryDetails(ctx, r.pool, caseID)
}

func (r *PostgresRepository) ResolveRecoveryDifference(ctx context.Context, caseID, differenceID, actorID uuid.UUID, requestID string) (RecoveryDetails, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return RecoveryDetails{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM device_recovery_cases WHERE id=$1 FOR UPDATE`, caseID).Scan(&status); errors.Is(err, pgx.ErrNoRows) {
		return RecoveryDetails{}, ErrNotFound
	} else if err != nil {
		return RecoveryDetails{}, fmt.Errorf("lock recovery case: %w", err)
	} else if status != "open" {
		return RecoveryDetails{}, fmt.Errorf("%w: recovery case is already resolved", ErrConflict)
	}
	var resolvedAt *time.Time
	if err := tx.QueryRow(ctx, `SELECT resolved_at FROM recovery_differences WHERE id=$1 AND recovery_case_id=$2 FOR UPDATE`, differenceID, caseID).Scan(&resolvedAt); errors.Is(err, pgx.ErrNoRows) {
		return RecoveryDetails{}, ErrNotFound
	} else if err != nil {
		return RecoveryDetails{}, fmt.Errorf("lock recovery difference: %w", err)
	}
	if resolvedAt == nil {
		now := r.now().UTC()
		if _, err := tx.Exec(ctx, `UPDATE recovery_differences SET resolved_at=$2 WHERE id=$1`, differenceID, now); err != nil {
			return RecoveryDetails{}, fmt.Errorf("resolve recovery difference: %w", err)
		}
		if err := insertAudit(ctx, tx, actorID, "pos.recovery_difference_resolved", "device_recovery_case", caseID, requestID, map[string]any{"difference_id": differenceID}); err != nil {
			return RecoveryDetails{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return RecoveryDetails{}, err
	}
	return getRecoveryDetails(ctx, r.pool, caseID)
}

func (r *PostgresRepository) UpdateRecoveryControls(ctx context.Context, caseID uuid.UUID, input RecoveryControlsInput, actorID uuid.UUID, requestID string) (RecoveryDetails, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return RecoveryDetails{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	command, err := tx.Exec(ctx, `UPDATE device_recovery_cases SET journal_status=$2,gaps_documented=$3,digital_payments_reviewed=$4,inventory_counted=$5,owner_approved=$6 WHERE id=$1 AND status='open'`, caseID, input.JournalStatus, input.GapsDocumented, input.DigitalPaymentsReviewed, input.InventoryCounted, input.OwnerApproved)
	if err != nil {
		return RecoveryDetails{}, fmt.Errorf("update recovery controls: %w", err)
	}
	if command.RowsAffected() != 1 {
		return RecoveryDetails{}, ErrNotFound
	}
	if err := insertAudit(ctx, tx, actorID, "pos.recovery_controls_updated", "device_recovery_case", caseID, requestID, map[string]any{"journal_status": input.JournalStatus, "owner_approved": input.OwnerApproved}); err != nil {
		return RecoveryDetails{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RecoveryDetails{}, err
	}
	return getRecoveryDetails(ctx, r.pool, caseID)
}

func (r *PostgresRepository) CloseRecovery(ctx context.Context, caseID uuid.UUID, actorID uuid.UUID, requestID string) (RecoveryDetails, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return RecoveryDetails{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := tx.QueryRow(ctx, `SELECT id FROM device_recovery_cases WHERE id=$1 FOR UPDATE`, caseID).Scan(&caseID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RecoveryDetails{}, ErrNotFound
		}
		return RecoveryDetails{}, err
	}
	details, err := getRecoveryDetails(ctx, tx, caseID)
	if err != nil {
		return RecoveryDetails{}, err
	}
	if details.Status != "open" || (details.JournalStatus != "imported" && details.JournalStatus != "unrecoverable") || !details.GapsDocumented || !details.DigitalPaymentsReviewed || !details.InventoryCounted || !details.OwnerApproved {
		return RecoveryDetails{}, fmt.Errorf("%w: complete every reconciliation control before closing", ErrConflict)
	}
	for _, difference := range details.Differences {
		if difference.ResolvedAt == nil {
			return RecoveryDetails{}, fmt.Errorf("%w: resolve every recovery difference before closing", ErrConflict)
		}
	}
	now := r.now().UTC()
	if details.AffectedShiftID != nil {
		var expected int64
		if err := tx.QueryRow(ctx, `SELECT opening_cash_cents + COALESCE(sum(CASE WHEN movement_type IN ('sale','income') THEN amount_cents ELSE -amount_cents END),0) FROM cash_shifts cs LEFT JOIN cash_movements cm ON cm.cash_shift_id=cs.id WHERE cs.id=$1 GROUP BY cs.opening_cash_cents`, *details.AffectedShiftID).Scan(&expected); err != nil {
			return RecoveryDetails{}, err
		}
		if _, err := tx.Exec(ctx, `UPDATE cash_shifts SET status='closed',closed_by=$2,counted_cash_cents=$3,expected_cash_cents=$4,difference_cents=$3::bigint-$4::bigint,closing_note='Cierre por conciliación',closed_at=$5 WHERE id=$1 AND status='reconciliation_required'`, *details.AffectedShiftID, actorID, details.CashCountCents, expected, now); err != nil {
			return RecoveryDetails{}, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE device_recovery_cases SET status='resolved',resolved_at=$2 WHERE id=$1`, caseID, now); err != nil {
		return RecoveryDetails{}, err
	}
	if err := insertAudit(ctx, tx, actorID, "pos.recovery_resolved", "device_recovery_case", caseID, requestID, map[string]any{"owner_approved": true}); err != nil {
		return RecoveryDetails{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RecoveryDetails{}, err
	}
	return getRecoveryDetails(ctx, r.pool, caseID)
}

func (r *PostgresRepository) ReconstructRecoverySale(ctx context.Context, caseID uuid.UUID, input RecoveryReconstructionInput, actorID uuid.UUID, requestID, payloadHash string) (Sale, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return Sale{}, fmt.Errorf("begin recovery reconstruction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var storedCaseID, storedSaleID uuid.UUID
	var storedSourceID *uuid.UUID
	var storedCertainty, storedEvidence string
	err = tx.QueryRow(ctx, `SELECT recovery_case_id,source_operation_id,sale_id,certainty,evidence FROM recovery_reconstructions WHERE reconciliation_operation_id=$1 FOR UPDATE`, input.ReconciliationOperationID).
		Scan(&storedCaseID, &storedSourceID, &storedSaleID, &storedCertainty, &storedEvidence)
	if err == nil {
		if storedCaseID != caseID || storedSaleID != input.Sale.OperationID || !sameOptionalUUID(storedSourceID, input.SourceOperationID) || storedCertainty != input.Certainty || storedEvidence != input.Evidence {
			return Sale{}, fmt.Errorf("%w: reconstruction id was reused with different content", ErrConflict)
		}
		sale, err := r.createSale(ctx, tx, input.Sale, actorID, requestID, payloadHash)
		if err != nil {
			return Sale{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Sale{}, mapWriteError("commit replayed recovery reconstruction", err)
		}
		return sale, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Sale{}, mapWriteError("find recovery reconstruction", err)
	}

	var status string
	var previousSessionID uuid.UUID
	var affectedShiftID *uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT status,previous_session_id,affected_shift_id FROM device_recovery_cases WHERE id=$1 FOR UPDATE`, caseID).Scan(&status, &previousSessionID, &affectedShiftID); errors.Is(err, pgx.ErrNoRows) {
		return Sale{}, ErrNotFound
	} else if err != nil {
		return Sale{}, mapWriteError("lock recovery case for reconstruction", err)
	}
	if status != "open" || affectedShiftID == nil || previousSessionID != input.Sale.DeviceSessionID || *affectedShiftID != input.Sale.CashShiftID {
		return Sale{}, fmt.Errorf("%w: reconstruction does not belong to the open recovery case", ErrConflict)
	}

	_, err = tx.Exec(ctx, `INSERT INTO recovery_reconstructions (id,recovery_case_id,reconciliation_operation_id,source_operation_id,sale_id,certainty,evidence,actor_id,request_id,created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, uuid.New(), caseID, input.ReconciliationOperationID, input.SourceOperationID, input.Sale.OperationID, input.Certainty, input.Evidence, actorID, requestID, r.now().UTC())
	if err != nil {
		return Sale{}, mapWriteError("reserve recovery reconstruction", err)
	}
	sale, err := r.createSale(ctx, tx, input.Sale, actorID, requestID, payloadHash)
	if err != nil {
		return Sale{}, err
	}
	if input.SourceOperationID != nil {
		var sequence int64
		var sourcePayloadHash, sourceStatus string
		var sourceSaleID *uuid.UUID
		err := tx.QueryRow(ctx, `SELECT sequence,payload_hash,status,result_sale_id FROM sync_operations WHERE operation_id=$1 FOR UPDATE`, *input.SourceOperationID).Scan(&sequence, &sourcePayloadHash, &sourceStatus, &sourceSaleID)
		if err == nil {
			if sourceStatus != "recovery_evidence" {
				return Sale{}, fmt.Errorf("%w: source operation is already being processed or applied", ErrConflict)
			}
			sourceEnvelope := OfflineOperation{OperationID: *input.SourceOperationID, Sequence: sequence, PayloadHash: sourcePayloadHash}
			ack := syncACK(sourceEnvelope, "acknowledged", sale.ID, uuid.Nil, true)
			ackJSON, _ := json.Marshal(ack)
			if _, err := tx.Exec(ctx, `UPDATE sync_operations SET status='acknowledged',result_sale_id=$2,ack=$3,acknowledged_at=$4 WHERE operation_id=$1`, *input.SourceOperationID, sale.ID, ackJSON, r.now().UTC()); err != nil {
				return Sale{}, fmt.Errorf("link recovery evidence to reconstructed sale: %w", err)
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return Sale{}, mapWriteError("lock source recovery operation", err)
		}
	}
	if err := insertAudit(ctx, tx, actorID, "pos.recovery_sale_reconstructed", "device_recovery_case", caseID, requestID, map[string]any{"sale_id": sale.ID, "certainty": input.Certainty, "source_operation_id": input.SourceOperationID}); err != nil {
		return Sale{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Sale{}, mapWriteError("commit recovery reconstruction", err)
	}
	return sale, nil
}

func sameOptionalUUID(left, right *uuid.UUID) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func getRecoveryDetails(ctx context.Context, q queryer, id uuid.UUID) (RecoveryDetails, error) {
	var d RecoveryDetails
	err := q.QueryRow(ctx, `SELECT rc.id,rc.previous_session_id,rc.replacement_session_id,rc.affected_shift_id,rc.status,rc.reason,rc.last_ack_sequence,rc.last_ack_hash,rc.cash_count_cents,rc.opened_at,rc.journal_status,rc.gaps_documented,rc.digital_payments_reviewed,rc.inventory_counted,rc.owner_approved,previous.fencing_version,COALESCE(cs.version,0),replacement.id,replacement.device_id,replacement.device_name,replacement.user_id,replacement.status,replacement.fencing_version,replacement.acquired_at FROM device_recovery_cases rc JOIN pos_device_sessions previous ON previous.id=rc.previous_session_id JOIN pos_device_sessions replacement ON replacement.id=rc.replacement_session_id LEFT JOIN cash_shifts cs ON cs.id=rc.affected_shift_id WHERE rc.id=$1`, id).Scan(&d.ID, &d.PreviousSessionID, &d.ReplacementSessionID, &d.AffectedShiftID, &d.Status, &d.Reason, &d.LastACKSequence, &d.LastACKHash, &d.CashCountCents, &d.OpenedAt, &d.JournalStatus, &d.GapsDocumented, &d.DigitalPaymentsReviewed, &d.InventoryCounted, &d.OwnerApproved, &d.PreviousFencingVersion, &d.AffectedShiftVersion, &d.ReplacementSession.ID, &d.ReplacementSession.DeviceID, &d.ReplacementSession.DeviceName, &d.ReplacementSession.UserID, &d.ReplacementSession.Status, &d.ReplacementSession.FencingVersion, &d.ReplacementSession.AcquiredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return RecoveryDetails{}, ErrNotFound
	}
	if err != nil {
		return RecoveryDetails{}, fmt.Errorf("get recovery: %w", err)
	}
	rows, err := q.Query(ctx, `SELECT id,kind,amount_cents,description,created_at,resolved_at FROM recovery_differences WHERE recovery_case_id=$1 ORDER BY created_at`, id)
	if err != nil {
		return RecoveryDetails{}, err
	}
	defer rows.Close()
	d.Differences = []RecoveryDifference{}
	for rows.Next() {
		var item RecoveryDifference
		if err := rows.Scan(&item.ID, &item.Kind, &item.AmountCents, &item.Description, &item.CreatedAt, &item.ResolvedAt); err != nil {
			return RecoveryDetails{}, err
		}
		d.Differences = append(d.Differences, item)
	}
	return d, rows.Err()
}
