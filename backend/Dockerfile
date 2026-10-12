FROM golang:1.26.9-alpine AS build

WORKDIR /src
RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum* ./
RUN go mod download
RUN GOBIN=/out go install github.com/pressly/goose/v3/cmd/goose@v3.27.3

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/seed ./cmd/seed

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S bakery \
    && adduser -S -G bakery bakery

WORKDIR /app
COPY --from=build /out/api /app/api
COPY --from=build /out/seed /app/seed
COPY --from=build /out/goose /usr/local/bin/goose
COPY migrations /app/migrations

USER bakery
EXPOSE 8080
CMD ["/app/api"]
