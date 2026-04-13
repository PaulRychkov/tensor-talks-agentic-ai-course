FROM golang:1.24-alpine AS build
WORKDIR /app
COPY go.mod ./
COPY go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o session-crud-service ./cmd/session-crud-service

FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=build /app/session-crud-service /usr/local/bin/session-crud-service
COPY config ./config
EXPOSE 8085
ENTRYPOINT ["/usr/local/bin/session-crud-service"]

