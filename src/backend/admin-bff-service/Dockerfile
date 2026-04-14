FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /admin-bff-service ./cmd/admin-bff-service

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /admin-bff-service .
COPY config/config.yaml ./config/

EXPOSE 8096
ENTRYPOINT ["/app/admin-bff-service"]
