FROM golang:1.24-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o mock-model-service ./cmd/mock-model-service

FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=build /app/mock-model-service /usr/local/bin/mock-model-service
COPY config ./config
EXPOSE 8084
ENTRYPOINT ["/usr/local/bin/mock-model-service"]
