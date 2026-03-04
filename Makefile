# Сборка сервера
.PHONY: build generate

build:
	go build -o bin/server ./cmd/server

# Генерация Go-кода из proto (нужен protoc, protoc-gen-go, protoc-gen-go-grpc).
# Код генерируется в пакет `internal/pb/proto` и используется во всём проекте.
generate:
	protoc --go_out=internal/pb/proto --go_opt=paths=source_relative \
		--go-grpc_out=internal/pb/proto --go-grpc_opt=paths=source_relative \
		-I. proto/telegram.proto

