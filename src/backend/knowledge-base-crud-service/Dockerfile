FROM golang:1.24-alpine AS build
WORKDIR /app
COPY go.mod ./
COPY go.sum* ./
RUN GOPROXY=https://proxy.golang.org,direct go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o knowledge-base-crud-service ./cmd/knowledge-base-crud-service

FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=build /app/knowledge-base-crud-service /usr/local/bin/knowledge-base-crud-service
COPY config ./config
EXPOSE 8090
ENTRYPOINT ["/usr/local/bin/knowledge-base-crud-service"]

