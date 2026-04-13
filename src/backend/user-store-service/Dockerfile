FROM golang:1.24-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -o user-store-service ./cmd/user-store-service

FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=build /app/user-store-service /usr/local/bin/user-store-service
COPY config ./config
EXPOSE 8082
ENTRYPOINT ["/usr/local/bin/user-store-service"]

