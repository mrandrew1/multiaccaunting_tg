package server

import (
	"google.golang.org/grpc"

	pb "pact-telegram/internal/pb/proto"
)

// Register регистрирует реализацию TelegramService в gRPC-сервере.
func Register(s *grpc.Server, svc pb.TelegramServiceServer) {
	pb.RegisterTelegramServiceServer(s, svc)
}
