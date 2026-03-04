# Сборка бинарника
FROM golang:1.26-alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /server ./cmd/server

# Минимальный образ для запуска
FROM alpine:3.20
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /server .

ENV GRPC_ADDR=:50051
EXPOSE 50051

ENTRYPOINT ["/app/server"]
