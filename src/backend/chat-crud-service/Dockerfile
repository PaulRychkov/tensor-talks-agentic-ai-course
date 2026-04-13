FROM golang:1.24-alpine AS build
WORKDIR /app
COPY go.mod ./
COPY go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o chat-crud-service ./cmd/chat-crud-service

FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=build /app/chat-crud-service /usr/local/bin/chat-crud-service
COPY config ./config
EXPOSE 8087
ENTRYPOINT ["/usr/local/bin/chat-crud-service"]

