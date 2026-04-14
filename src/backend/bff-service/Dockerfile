FROM golang:1.24-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN GOPROXY=https://goproxy.io,https://proxy.golang.org,direct go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o bff-service ./cmd/bff-service

FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=build /app/bff-service /usr/local/bin/bff-service
COPY config ./config
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/bff-service"]

