FROM node:24-alpine AS frontend-build

WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.26.9-alpine AS backend-build

WORKDIR /src/backend
RUN apk add --no-cache ca-certificates git
COPY backend/go.mod backend/go.sum ./
RUN go mod download
RUN GOBIN=/out go install github.com/pressly/goose/v3/cmd/goose@v3.27.3
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/seed ./cmd/seed

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S bakery \
    && adduser -S -G bakery bakery

WORKDIR /app
COPY --from=backend-build /out/api /app/api
COPY --from=backend-build /out/seed /app/seed
COPY --from=backend-build /out/goose /usr/local/bin/goose
COPY --from=backend-build /src/backend/migrations /app/migrations
COPY --from=frontend-build /src/frontend/dist /app/frontend

USER bakery
EXPOSE 10000

CMD ["/bin/sh", "-c", "goose -dir /app/migrations postgres \"$DATABASE_URL\" up && /app/seed && exec /app/api"]
