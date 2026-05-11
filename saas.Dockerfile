FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY config.yaml ./

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/saas ./cmd/saas

FROM alpine:3.22

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /out/saas /app/saas
COPY config.yaml /app/config.yaml

EXPOSE 8080

ENTRYPOINT ["/app/saas", "-config", "/app/config.yaml"]
