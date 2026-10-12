.PHONY: dev down migrate seed test test-backend test-frontend test-db test-db-down lint logs

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

test-db-down:
	docker compose --profile test stop db-test

lint:
	cd backend && gofmt -l . && go vet ./...
	cd frontend && npm run lint && npm run typecheck

logs:
	docker compose logs -f backend frontend db
