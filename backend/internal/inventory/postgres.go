package inventory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

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

func (r *PostgresRepository) ListBalances(ctx context.Context, filter BalanceFilter) ([]Balance, int, error) {
	const countQuery = `
		SELECT count(*)
		FROM inventory_balances b
		JOIN products p ON p.id = b.product_id
		WHERE ($1 = '' OR p.name ILIKE '%' || $1 || '%')`
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, filter.Query).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count inventory balances: %w", err)
	}

	const query = `
		SELECT b.product_id, p.name, p.base_unit, b.quantity::text, b.updated_at
		FROM inventory_balances b
		JOIN products p ON p.id = b.product_id
		WHERE ($1 = '' OR p.name ILIKE '%' || $1 || '%')
		ORDER BY lower(p.name), p.id
		LIMIT $2 OFFSET $3`
	rows, err := r.pool.Query(ctx, query, filter.Query, filter.Limit, filter.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list inventory balances: %w", err)
	}
	defer rows.Close()

	balances := make([]Balance, 0)
	for rows.Next() {
		var balance Balance
		var quantity string
		if err := rows.Scan(&balance.ProductID, &balance.ProductName, &balance.BaseUnit, &quantity, &balance.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan inventory balance: %w", err)
		}
		balance.Quantity, err = ParseQuantity(quantity)
		if err != nil {
			return nil, 0, fmt.Errorf("parse inventory balance: %w", err)
		}
		balances = append(balances, balance)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate inventory balances: %w", err)
	}
	return balances, total, nil
}

func (r *PostgresRepository) ListMovements(ctx context.Context, filter MovementFilter) ([]Movement, int, error) {
	productID := nullableUUID(filter.ProductID)
	movementType := nullableMovementType(filter.MovementType)
	const countQuery = `
		SELECT count(*)
		FROM inventory_movements
		WHERE ($1::uuid IS NULL OR product_id = $1)
		  AND ($2::text IS NULL OR movement_type = $2)`
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, productID, movementType).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count inventory movements: %w", err)
	}

	const query = `
		SELECT m.id, m.product_id, p.name, p.base_unit, m.quantity_delta::text,
		       m.movement_type, m.reference_type, COALESCE(m.reference_id::text, ''),
		       m.actor_id, m.request_id, COALESCE(m.reason, ''), m.occurred_at, m.created_at,
		       COALESCE(m.payload_hash, '')
		FROM inventory_movements m
		JOIN products p ON p.id = m.product_id
		WHERE ($1::uuid IS NULL OR m.product_id = $1)
		  AND ($2::text IS NULL OR m.movement_type = $2)
		ORDER BY m.occurred_at DESC, m.id DESC
		LIMIT $3 OFFSET $4`
	rows, err := r.pool.Query(ctx, query, productID, movementType, filter.Limit, filter.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list inventory movements: %w", err)
	}
	defer rows.Close()

	movements := make([]Movement, 0)
	for rows.Next() {
		movement, _, err := scanMovement(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan inventory movement: %w", err)
		}
		movements = append(movements, movement)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate inventory movements: %w", err)
	}
	return movements, total, nil
}

func (r *PostgresRepository) RecordMovement(
	ctx context.Context,
	input RecordMovementInput,
	actorID uuid.UUID,
	requestID string,
	payloadHash string,
) (RecordResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return RecordResult{}, fmt.Errorf("begin inventory movement: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var baseUnit BaseUnit
	if err := tx.QueryRow(ctx, `SELECT base_unit FROM products WHERE id = $1`, input.ProductID).Scan(&baseUnit); errors.Is(err, pgx.ErrNoRows) {
		return RecordResult{}, fmt.Errorf("%w: product does not exist", ErrNotFound)
	} else if err != nil {
		return RecordResult{}, fmt.Errorf("find inventory product: %w", err)
	}
	if baseUnit == BaseUnitUnit && !input.QuantityDelta.IsInteger() {
		return RecordResult{}, fmt.Errorf("%w: unit products require integer quantities", ErrInvalid)
	}

	now := r.now().UTC()
	const insert = `
		INSERT INTO inventory_movements (
			id, product_id, quantity_delta, movement_type, reference_type, reference_id,
			actor_id, request_id, reason, occurred_at, created_at, payload_hash
		)
		VALUES ($1, $2, $3::numeric, $4, $5, $6, $7, $8, NULLIF($9, ''), $10, $10, $11)
		ON CONFLICT (id) DO NOTHING`
	tag, err := tx.Exec(ctx, insert, input.ID, input.ProductID, input.QuantityDelta.String(), input.MovementType,
		input.ReferenceType, input.ReferenceID, actorID, requestID, input.Reason, now, payloadHash)
	if err != nil {
		return RecordResult{}, mapWriteError("insert inventory movement", err)
	}

	movement, storedHash, err := getMovement(ctx, tx, input.ID)
	if err != nil {
		return RecordResult{}, err
	}
	if storedHash != payloadHash {
		return RecordResult{}, fmt.Errorf("%w: movement id was already used with a different payload", ErrConflict)
	}
	balance, err := getBalance(ctx, tx, input.ProductID)
	if err != nil {
		return RecordResult{}, err
	}
	if tag.RowsAffected() == 0 {
		return RecordResult{Movement: movement, Balance: balance, Replayed: true}, nil
	}

	if err := insertAudit(ctx, tx, actorID, "inventory.movement_recorded", "inventory_movement", input.ID, requestID, movement); err != nil {
		return RecordResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RecordResult{}, fmt.Errorf("commit inventory movement: %w", err)
	}
	return RecordResult{Movement: movement, Balance: balance}, nil
}

func (r *PostgresRepository) ReconstructBalance(ctx context.Context, productID uuid.UUID) (Quantity, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM products WHERE id = $1)`, productID).Scan(&exists); err != nil {
		return Quantity{}, fmt.Errorf("find inventory product: %w", err)
	}
	if !exists {
		return Quantity{}, fmt.Errorf("%w: product does not exist", ErrNotFound)
	}
	var quantity string
	if err := r.pool.QueryRow(ctx, `SELECT COALESCE(sum(quantity_delta), 0)::text FROM inventory_movements WHERE product_id = $1`, productID).Scan(&quantity); err != nil {
		return Quantity{}, fmt.Errorf("reconstruct inventory balance: %w", err)
	}
	parsed, err := ParseQuantity(quantity)
	if err != nil {
		return Quantity{}, fmt.Errorf("parse reconstructed inventory balance: %w", err)
	}
	return parsed, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func getMovement(ctx context.Context, tx pgx.Tx, id uuid.UUID) (Movement, string, error) {
	const query = `
		SELECT m.id, m.product_id, p.name, p.base_unit, m.quantity_delta::text,
		       m.movement_type, m.reference_type, COALESCE(m.reference_id::text, ''),
		       m.actor_id, m.request_id, COALESCE(m.reason, ''), m.occurred_at, m.created_at,
		       COALESCE(m.payload_hash, '')
		FROM inventory_movements m
		JOIN products p ON p.id = m.product_id
		WHERE m.id = $1`
	movement, payloadHash, err := scanMovement(tx.QueryRow(ctx, query, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Movement{}, "", fmt.Errorf("%w: movement does not exist", ErrNotFound)
	}
	if err != nil {
		return Movement{}, "", fmt.Errorf("get inventory movement: %w", err)
	}
	return movement, payloadHash, nil
}

func scanMovement(row rowScanner) (Movement, string, error) {
	var movement Movement
	var quantity, referenceID, payloadHash string
	if err := row.Scan(
		&movement.ID,
		&movement.ProductID,
		&movement.ProductName,
		&movement.BaseUnit,
		&quantity,
		&movement.MovementType,
		&movement.ReferenceType,
		&referenceID,
		&movement.ActorID,
		&movement.RequestID,
		&movement.Reason,
		&movement.OccurredAt,
		&movement.CreatedAt,
		&payloadHash,
	); err != nil {
		return Movement{}, "", err
	}
	parsedQuantity, err := ParseQuantity(quantity)
	if err != nil {
		return Movement{}, "", err
	}
	movement.QuantityDelta = parsedQuantity
	if referenceID != "" {
		parsedReference, err := uuid.Parse(referenceID)
		if err != nil {
			return Movement{}, "", err
		}
		movement.ReferenceID = &parsedReference
	}
	return movement, payloadHash, nil
}

func getBalance(ctx context.Context, tx pgx.Tx, productID uuid.UUID) (Balance, error) {
	const query = `
		SELECT b.product_id, p.name, p.base_unit, b.quantity::text, b.updated_at
		FROM inventory_balances b
		JOIN products p ON p.id = b.product_id
		WHERE b.product_id = $1`
	var balance Balance
	var quantity string
	if err := tx.QueryRow(ctx, query, productID).Scan(&balance.ProductID, &balance.ProductName, &balance.BaseUnit, &quantity, &balance.UpdatedAt); errors.Is(err, pgx.ErrNoRows) {
		return Balance{}, fmt.Errorf("%w: balance does not exist", ErrNotFound)
	} else if err != nil {
		return Balance{}, fmt.Errorf("get inventory balance: %w", err)
	}
	parsed, err := ParseQuantity(quantity)
	if err != nil {
		return Balance{}, fmt.Errorf("parse inventory balance: %w", err)
	}
	balance.Quantity = parsed
	return balance, nil
}

func insertAudit(ctx context.Context, tx pgx.Tx, actorID uuid.UUID, action, entityType string, entityID uuid.UUID, requestID string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("encode inventory audit data: %w", err)
	}
	const query = `
		INSERT INTO audit_events (id, actor_id, action, entity_type, entity_id, request_id, data)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)`
	if _, err := tx.Exec(ctx, query, uuid.New(), actorID, action, entityType, entityID, requestID, string(payload)); err != nil {
		return fmt.Errorf("insert inventory audit event: %w", err)
	}
	return nil
}

func mapWriteError(operation string, err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return fmt.Errorf("%w: duplicate inventory operation", ErrConflict)
		case "23503", "23514":
			return fmt.Errorf("%w: inventory constraint failed", ErrInvalid)
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func nullableUUID(value *uuid.UUID) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableMovementType(value *MovementType) any {
	if value == nil {
		return nil
	}
	return string(*value)
}
