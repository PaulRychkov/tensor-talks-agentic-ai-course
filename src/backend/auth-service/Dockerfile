FROM golang:1.24-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN GOPROXY=https://goproxy.io,https://proxy.golang.org,direct go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o auth-service ./cmd/auth-service

FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=build /app/auth-service /usr/local/bin/auth-service
COPY config ./config
EXPOSE 8081
ENTRYPOINT ["/usr/local/bin/auth-service"]

