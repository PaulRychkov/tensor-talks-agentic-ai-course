FROM golang:1.24-alpine AS build
WORKDIR /app
COPY go.mod go.sum* ./
RUN GOPROXY=https://goproxy.io,https://proxy.golang.org,direct go mod download
COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -o session-service ./cmd/session-service

FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=build /app/session-service /usr/local/bin/session-service
COPY config ./config
EXPOSE 8083
ENTRYPOINT ["/usr/local/bin/session-service"]
