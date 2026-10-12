.PHONY: dev down migrate seed test test-backend test-frontend test-integration test-db test-db-down lint logs

dev:
	docker compose up --build

down:
	docker compose down

migrate:
	docker compose run --rm backend goose -dir /app/migrations postgres "$${DATABASE_URL}" up

seed:
	docker compose --profile tools run --rm seed

test: test-backend test-frontend

test-backend:
	cd backend && go test ./...

test-frontend:
	cd frontend && npm test -- --run

test-db:
	docker compose --profile test up -d --wait db-test

test-integration: test-db
	docker compose --profile test run --rm --build --no-deps backend goose -dir /app/migrations postgres "postgres://bakery:bakery_test_only@db-test:5432/bakery_pos_test?sslmode=disable" up
	cd backend && TEST_DATABASE_URL="postgres://bakery:bakery_test_only@localhost:5433/bakery_pos_test?sslmode=disable" go test -count=1 -p=1 -tags=integration ./internal/...

test-db-down:
	docker compose --profile test stop db-test

lint:
	cd backend && gofmt -l . && go vet ./...
	cd frontend && npm run lint && npm run typecheck

logs:
	docker compose logs -f backend frontend db
