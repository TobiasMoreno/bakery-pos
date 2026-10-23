package pos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *PostgresRepository) ValidateOfflinePreparation(ctx context.Context, input OfflineAuthorizationRequest, actorID uuid.UUID) (DeviceSession, error) {
	var session DeviceSession
	var shiftVersion, catalogVersion int64
	var shiftStatus string
	err := r.pool.QueryRow(ctx, `
		SELECT ds.id, ds.device_id, ds.device_name, ds.user_id, ds.status, ds.fencing_version, ds.acquired_at,
		       cs.status, cs.version, cat.version
		FROM pos_device_sessions ds
		JOIN cash_shifts cs ON cs.id = $2 AND cs.device_session_id = ds.id
		CROSS JOIN catalog_state cat
		WHERE ds.id = $1`, input.DeviceSessionID, input.CashShiftID).Scan(
		&session.ID, &session.DeviceID, &session.DeviceName, &session.UserID, &session.Status,
		&session.FencingVersion, &session.AcquiredAt, &shiftStatus, &shiftVersion, &catalogVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeviceSession{}, ErrNoActiveDevice
	}
	if err != nil {
		return DeviceSession{}, fmt.Errorf("validate offline preparation: %w", err)
	}
	if session.Status != "active" || session.FencingVersion != input.FencingVersion || session.UserID != actorID ||
		shiftStatus != "open" || shiftVersion != input.ShiftVersion || catalogVersion != input.CatalogVersion {
		return DeviceSession{}, fmt.Errorf("%w: refresh the active device, shift and catalog before enabling offline sales", ErrConflict)
	}
	return session, nil
}

func (r *PostgresRepository) ReserveSyncOperation(ctx context.Context, op OfflineOperation, actorID uuid.UUID, requestID string) (SyncReservation, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return SyncReservation{}, fmt.Errorf("begin sync reservation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var storedHash, storedPreviousHash, status string
	var storedSequence int64
	var storedDeviceID, storedSessionID uuid.UUID
	var storedFencingVersion int64
	var resultSaleID *uuid.UUID
	var resultShiftID *uuid.UUID
	err = tx.QueryRow(ctx, `SELECT payload_hash, previous_hash, sequence, device_id, device_session_id, fencing_version, status, result_sale_id, result_shift_id FROM sync_operations WHERE operation_id = $1 FOR UPDATE`, op.OperationID).
		Scan(&storedHash, &storedPreviousHash, &storedSequence, &storedDeviceID, &storedSessionID, &storedFencingVersion, &status, &resultSaleID, &resultShiftID)
	if err == nil {
		if storedHash != op.PayloadHash || storedPreviousHash != op.PreviousHash || storedSequence != op.Sequence || storedDeviceID != op.DeviceID || storedSessionID != op.DeviceSessionID || storedFencingVersion != op.FencingVersion {
			return SyncReservation{}, fmt.Errorf("%w: operation id was reused with different content", ErrSyncConflict)
		}
		if status == "recovery_evidence" {
			var linkedSaleID uuid.UUID
			linkErr := tx.QueryRow(ctx, `SELECT sale_id FROM recovery_reconstructions WHERE source_operation_id=$1`, op.OperationID).Scan(&linkedSaleID)
			if linkErr == nil {
				ack := syncACK(op, "acknowledged", linkedSaleID, uuid.Nil, true)
				ackJSON, _ := json.Marshal(ack)
				now := r.now().UTC()
				if _, err := tx.Exec(ctx, `UPDATE sync_operations SET status='acknowledged',result_sale_id=$2,ack=$3,acknowledged_at=$4 WHERE operation_id=$1`, op.OperationID, linkedSaleID, ackJSON, now); err != nil {
					return SyncReservation{}, fmt.Errorf("link recovery evidence to reconstructed sale: %w", err)
				}
				if err := insertAudit(ctx, tx, actorID, "pos.recovery_evidence_linked", "sync_operation", op.OperationID, requestID, map[string]any{"sale_id": linkedSaleID}); err != nil {
					return SyncReservation{}, err
				}
				if err := tx.Commit(ctx); err != nil {
					return SyncReservation{}, mapWriteError("commit linked recovery evidence", err)
				}
				return SyncReservation{ACK: &ack}, nil
			}
			if !errors.Is(linkErr, pgx.ErrNoRows) {
				return SyncReservation{}, fmt.Errorf("find reconstruction for recovery evidence: %w", linkErr)
			}
		}
		if status == "acknowledged" || status == "recovery_evidence" {
			return SyncReservation{ACK: &SyncACK{OperationID: op.OperationID, Sequence: op.Sequence, PayloadHash: op.PayloadHash, Status: status, SaleID: resultSaleID, ShiftID: resultShiftID, Replayed: true}}, nil
		}
		return SyncReservation{}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return SyncReservation{}, fmt.Errorf("find synchronized operation: %w", err)
	}

	var sessionStatus string
	var deviceID uuid.UUID
	var fencingVersion int64
	if err := tx.QueryRow(ctx, `SELECT device_id, status, fencing_version FROM pos_device_sessions WHERE id = $1 FOR UPDATE`, op.DeviceSessionID).
		Scan(&deviceID, &sessionStatus, &fencingVersion); errors.Is(err, pgx.ErrNoRows) {
		return SyncReservation{}, ErrNoActiveDevice
	} else if err != nil {
		return SyncReservation{}, fmt.Errorf("lock synchronized device: %w", err)
	}
	if deviceID != op.DeviceID || fencingVersion != op.FencingVersion {
		return SyncReservation{}, fmt.Errorf("%w: device envelope does not match its session", ErrSyncConflict)
	}

	lastSequence, lastHash, err := lockSyncState(ctx, tx, op.DeviceSessionID)
	if err != nil {
		return SyncReservation{}, err
	}
	if op.Sequence != lastSequence+1 || op.PreviousHash != lastHash {
		return SyncReservation{}, fmt.Errorf("%w: expected sequence %d and previous hash %q", ErrSyncConflict, lastSequence+1, lastHash)
	}
	operationStatus := "processing"
	evidence := false
	var matchedSaleID *uuid.UUID
	if sessionStatus == "recovery_required" {
		_ = tx.QueryRow(ctx, `SELECT sale_id FROM recovery_reconstructions WHERE source_operation_id = $1`, op.OperationID).Scan(&matchedSaleID)
		if matchedSaleID != nil {
			operationStatus = "acknowledged"
		} else {
			operationStatus = "recovery_evidence"
			evidence = true
		}
	} else if sessionStatus != "active" {
		return SyncReservation{}, ErrNoActiveDevice
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO sync_operations (operation_id, device_id, device_session_id, fencing_version, sequence,
		    operation_type, previous_hash, payload_hash, payload, status, result_sale_id, created_at_local, timezone, acknowledged_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, CASE WHEN $10='acknowledged' THEN now() ELSE NULL END)`,
		op.OperationID, op.DeviceID, op.DeviceSessionID, op.FencingVersion, op.Sequence,
		op.OperationType, op.PreviousHash, op.PayloadHash, op.Payload, operationStatus, matchedSaleID, op.CreatedAtLocal.UTC(), op.Timezone)
	if err != nil {
		return SyncReservation{}, mapWriteError("reserve synchronized operation", err)
	}
	if evidence || matchedSaleID != nil {
		if _, err := tx.Exec(ctx, `UPDATE sync_session_state SET last_sequence = $2, last_hash = $3, updated_at = $4 WHERE device_session_id = $1`, op.DeviceSessionID, op.Sequence, op.PayloadHash, r.now().UTC()); err != nil {
			return SyncReservation{}, fmt.Errorf("advance recovery evidence chain: %w", err)
		}
	}
	if err := insertAudit(ctx, tx, actorID, "sync.operation_received", "sync_operation", op.OperationID, requestID, map[string]any{
		"sequence": op.Sequence, "status": operationStatus,
	}); err != nil {
		return SyncReservation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SyncReservation{}, mapWriteError("commit sync reservation", err)
	}
	if matchedSaleID != nil {
		ack := syncACK(op, "acknowledged", *matchedSaleID, uuid.Nil, true)
		return SyncReservation{ACK: &ack}, nil
	}
	return SyncReservation{Evidence: evidence}, nil
}

func (r *PostgresRepository) CompleteSyncOperation(ctx context.Context, op OfflineOperation, saleID, shiftID uuid.UUID, requestID string) (SyncACK, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return SyncACK{}, fmt.Errorf("begin sync completion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var status, storedHash string
	var storedSaleID *uuid.UUID
	var storedShiftID *uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT status, payload_hash, result_sale_id, result_shift_id FROM sync_operations WHERE operation_id = $1 FOR UPDATE`, op.OperationID).
		Scan(&status, &storedHash, &storedSaleID, &storedShiftID); err != nil {
		return SyncACK{}, fmt.Errorf("lock synchronized operation: %w", err)
	}
	if storedHash != op.PayloadHash {
		return SyncACK{}, ErrSyncConflict
	}
	if status == "acknowledged" || status == "recovery_evidence" {
		if status == "recovery_evidence" {
			saleID = uuid.Nil
		} else if storedSaleID != nil {
			saleID = *storedSaleID
		}
		if storedShiftID != nil {
			shiftID = *storedShiftID
		}
		return syncACK(op, status, saleID, shiftID, true), nil
	}
	if saleID == uuid.Nil && shiftID == uuid.Nil {
		return SyncACK{}, fmt.Errorf("%w: an applied operation requires a result", ErrInvalid)
	}
	ack := syncACK(op, "acknowledged", saleID, shiftID, false)
	ackJSON, _ := json.Marshal(ack)
	now := r.now().UTC()
	if _, err := tx.Exec(ctx, `UPDATE sync_operations SET status = 'acknowledged', result_sale_id = NULLIF($2::uuid, $5::uuid), result_shift_id = NULLIF($3::uuid, $5::uuid), ack = $4, acknowledged_at = $6 WHERE operation_id = $1`, op.OperationID, saleID, shiftID, ackJSON, uuid.Nil, now); err != nil {
		return SyncACK{}, fmt.Errorf("store synchronized ACK: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE sync_session_state SET last_sequence = $2, last_hash = $3, updated_at = $4 WHERE device_session_id = $1`, op.DeviceSessionID, op.Sequence, op.PayloadHash, now); err != nil {
		return SyncACK{}, fmt.Errorf("advance synchronized chain: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return SyncACK{}, mapWriteError("commit sync completion", err)
	}
	return ack, nil
}

func (r *PostgresRepository) SyncStatus(ctx context.Context, sessionID uuid.UUID) (SyncStatus, error) {
	status := SyncStatus{DeviceSessionID: sessionID}
	err := r.pool.QueryRow(ctx, `SELECT last_sequence, last_hash FROM sync_session_state WHERE device_session_id = $1`, sessionID).Scan(&status.LastSequence, &status.LastHash)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return SyncStatus{}, fmt.Errorf("read synchronization status: %w", err)
	}
	var recoveryCaseID uuid.UUID
	if err := r.pool.QueryRow(ctx, `SELECT id FROM device_recovery_cases WHERE previous_session_id = $1 AND status = 'open'`, sessionID).Scan(&recoveryCaseID); err == nil {
		status.RecoveryCaseID = &recoveryCaseID
	}
	return status, nil
}

func (r *PostgresRepository) ForceRecovery(ctx context.Context, input ForceRecoveryInput, actorID uuid.UUID, requestID string) (RecoveryCase, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return RecoveryCase{}, fmt.Errorf("begin forced recovery: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var previous DeviceSession
	if err := tx.QueryRow(ctx, `SELECT id, device_id, device_name, user_id, status, fencing_version, acquired_at FROM pos_device_sessions WHERE id = $1 FOR UPDATE`, input.PreviousSessionID).
		Scan(&previous.ID, &previous.DeviceID, &previous.DeviceName, &previous.UserID, &previous.Status, &previous.FencingVersion, &previous.AcquiredAt); errors.Is(err, pgx.ErrNoRows) {
		return RecoveryCase{}, ErrNotFound
	} else if err != nil {
		return RecoveryCase{}, fmt.Errorf("lock previous device session: %w", err)
	}
	if previous.Status != "active" {
		return RecoveryCase{}, fmt.Errorf("%w: only the active device can be recovered", ErrConflict)
	}
	lastSequence, lastHash, err := lockSyncState(ctx, tx, previous.ID)
	if err != nil {
		return RecoveryCase{}, err
	}
	now := r.now().UTC()
	if _, err := tx.Exec(ctx, `UPDATE pos_device_sessions SET status = 'recovery_required', released_at = $2, release_request_id = $3 WHERE id = $1`, previous.ID, now, requestID); err != nil {
		return RecoveryCase{}, fmt.Errorf("fence previous device: %w", err)
	}
	var affectedShiftID *uuid.UUID
	_ = tx.QueryRow(ctx, `UPDATE cash_shifts SET status = 'reconciliation_required', version = version + 1 WHERE device_session_id = $1 AND status = 'open' RETURNING id`, previous.ID).Scan(&affectedShiftID)
	var fencingVersion int64
	if err := tx.QueryRow(ctx, `UPDATE pos_state SET fencing_version = fencing_version + 1, updated_at = $1 WHERE singleton = TRUE RETURNING fencing_version`, now).Scan(&fencingVersion); err != nil {
		return RecoveryCase{}, fmt.Errorf("advance recovery fencing version: %w", err)
	}
	replacement := DeviceSession{ID: uuid.New(), DeviceID: input.DeviceID, DeviceName: input.DeviceName, UserID: actorID, Status: "active", FencingVersion: fencingVersion, AcquiredAt: now}
	if _, err := tx.Exec(ctx, `INSERT INTO pos_device_sessions (id, device_id, device_name, user_id, status, fencing_version, acquired_at) VALUES ($1,$2,$3,$4,'active',$5,$6)`, replacement.ID, replacement.DeviceID, replacement.DeviceName, actorID, replacement.FencingVersion, now); err != nil {
		return RecoveryCase{}, mapWriteError("insert recovery device", err)
	}
	recovery := RecoveryCase{ID: uuid.New(), PreviousSessionID: previous.ID, ReplacementSessionID: replacement.ID, AffectedShiftID: affectedShiftID, Status: "open", Reason: input.Reason, LastACKSequence: lastSequence, LastACKHash: lastHash, CashCountCents: input.CashCountCents, OpenedAt: now, PreviousFencingVersion: previous.FencingVersion, ReplacementSession: replacement}
	if affectedShiftID != nil {
		_ = tx.QueryRow(ctx, `SELECT version FROM cash_shifts WHERE id=$1`, *affectedShiftID).Scan(&recovery.AffectedShiftVersion)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO device_recovery_cases (id, previous_session_id, replacement_session_id, affected_shift_id, owner_id, status, reason, last_ack_sequence, last_ack_hash, cash_count_cents, opened_at) VALUES ($1,$2,$3,$4,$5,'open',$6,$7,$8,$9,$10)`, recovery.ID, recovery.PreviousSessionID, recovery.ReplacementSessionID, recovery.AffectedShiftID, actorID, recovery.Reason, lastSequence, lastHash, recovery.CashCountCents, now); err != nil {
		return RecoveryCase{}, mapWriteError("insert recovery case", err)
	}
	if affectedShiftID != nil {
		if _, err := tx.Exec(ctx, `UPDATE cash_shifts SET recovery_case_id = $2 WHERE id = $1`, *affectedShiftID, recovery.ID); err != nil {
			return RecoveryCase{}, fmt.Errorf("link recovery shift: %w", err)
		}
	}
	if err := insertAudit(ctx, tx, actorID, "pos.recovery_opened", "device_recovery_case", recovery.ID, requestID, map[string]any{"previous_session_id": previous.ID, "replacement_session_id": replacement.ID, "last_ack_sequence": lastSequence}); err != nil {
		return RecoveryCase{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RecoveryCase{}, mapWriteError("commit forced recovery", err)
	}
	return recovery, nil
}

func lockSyncState(ctx context.Context, tx pgx.Tx, sessionID uuid.UUID) (int64, string, error) {
	if _, err := tx.Exec(ctx, `INSERT INTO sync_session_state (device_session_id) VALUES ($1) ON CONFLICT DO NOTHING`, sessionID); err != nil {
		return 0, "", fmt.Errorf("initialize synchronization state: %w", err)
	}
	var sequence int64
	var hash string
	if err := tx.QueryRow(ctx, `SELECT last_sequence, last_hash FROM sync_session_state WHERE device_session_id = $1 FOR UPDATE`, sessionID).Scan(&sequence, &hash); err != nil {
		return 0, "", fmt.Errorf("lock synchronization state: %w", err)
	}
	return sequence, hash, nil
}

func syncACK(op OfflineOperation, status string, saleID, shiftID uuid.UUID, replayed bool) SyncACK {
	ack := SyncACK{OperationID: op.OperationID, Sequence: op.Sequence, PayloadHash: op.PayloadHash, Status: status, Replayed: replayed}
	if saleID != uuid.Nil {
		ack.SaleID = &saleID
	}
	if shiftID != uuid.Nil {
		ack.ShiftID = &shiftID
	}
	return ack
}
