FROM golang:1.24-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN GOPROXY=https://goproxy.io,https://proxy.golang.org,direct go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o dialogue-aggregator ./cmd/dialogue-aggregator

FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=build /app/dialogue-aggregator /usr/local/bin/dialogue-aggregator
COPY config ./config
EXPOSE 8084
ENTRYPOINT ["/usr/local/bin/dialogue-aggregator"]
