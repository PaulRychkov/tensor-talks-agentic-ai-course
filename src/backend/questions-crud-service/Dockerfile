FROM golang:1.24-alpine AS build
WORKDIR /app
COPY go.mod ./
COPY go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o questions-crud-service ./cmd/questions-crud-service

FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=build /app/questions-crud-service /usr/local/bin/questions-crud-service
COPY config ./config
EXPOSE 8091
ENTRYPOINT ["/usr/local/bin/questions-crud-service"]

