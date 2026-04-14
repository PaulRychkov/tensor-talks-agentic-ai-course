FROM golang:1.24-alpine AS build
WORKDIR /app
COPY go.mod ./
COPY go.sum* ./
RUN GOPROXY=https://goproxy.io,https://proxy.golang.org,direct go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o results-crud-service ./cmd/results-crud-service

FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=build /app/results-crud-service /usr/local/bin/results-crud-service
COPY config ./config
EXPOSE 8088
ENTRYPOINT ["/usr/local/bin/results-crud-service"]

