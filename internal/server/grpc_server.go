package server

import (
	"context"
	"errors"
	"strings"

	"github.com/gotd/td/tgerr"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "pact-telegram/internal/pb/proto"
	"pact-telegram/internal/telegram"
)

// sessionManager описывает зависимости GRPCServer от менеджера сессий.
// Выделен в интерфейс, чтобы в юнит-тестах можно было подменять реализацию
// и не ходить в реальный Telegram.
type sessionManager interface {
	CreateSession(ctx context.Context) (sessionID string, qr string, err error)
	DeleteSession(ctx context.Context, id string) error
	GetSession(id string) (*telegram.Session, bool)
}

// GRPCServer реализует gRPC-сервис TelegramService и делегирует работу SessionManager.
type GRPCServer struct {
	pb.UnimplementedTelegramServiceServer

	sessions sessionManager
}

// NewGRPCServer создаёт новый экземпляр gRPC-сервера.
func NewGRPCServer(m *telegram.SessionManager) *GRPCServer {
	return &GRPCServer{
		sessions: m,
	}
}

func (s *GRPCServer) CreateSession(ctx context.Context, _ *pb.CreateSessionRequest) (*pb.CreateSessionResponse, error) {
	id, qr, err := s.sessions.CreateSession(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil, status.Error(codes.Canceled, ctx.Err().Error())
		}
		return nil, status.Errorf(codes.Internal, "create session: %v", err)
	}
	return &pb.CreateSessionResponse{
		SessionId: &id,
		QrCode:    &qr,
	}, nil
}

func (s *GRPCServer) DeleteSession(ctx context.Context, req *pb.DeleteSessionRequest) (*pb.DeleteSessionResponse, error) {
	if err := s.sessions.DeleteSession(ctx, req.GetSessionId()); err != nil {
		return nil, status.Errorf(codes.Internal, "delete session: %v", err)
	}
	return &pb.DeleteSessionResponse{}, nil
}

func (s *GRPCServer) SendMessage(ctx context.Context, req *pb.SendMessageRequest) (*pb.SendMessageResponse, error) {
	session, ok := s.sessions.GetSession(req.GetSessionId())
	if !ok {
		return nil, status.Error(codes.NotFound, "session not found")
	}
	if !session.IsReady() {
		return nil, status.Error(codes.FailedPrecondition, "session not authorized yet, scan QR first")
	}
	msgID, err := session.SendMessage(ctx, req.GetPeer(), req.GetText())
	if err != nil {
		return nil, mapTelegramError(err)
	}
	return &pb.SendMessageResponse{
		MessageId: &msgID,
	}, nil
}

func (s *GRPCServer) SubscribeMessages(req *pb.SubscribeMessagesRequest, stream pb.TelegramService_SubscribeMessagesServer) error {
	session, ok := s.sessions.GetSession(req.GetSessionId())
	if !ok {
		return status.Error(codes.NotFound, "session not found")
	}
	if !session.IsReady() {
		return status.Error(codes.FailedPrecondition, "session not authorized yet, scan QR first")
	}
	ch, unsubscribe := session.Subscribe()
	defer unsubscribe()

	ctx := stream.Context()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case mu, ok := <-ch:
			if !ok {
				return nil
			}
			if mu == nil {
				continue
			}
			mid, from, text, ts := mu.MessageID, mu.From, mu.Text, mu.Timestamp
			if err := stream.Send(&pb.MessageUpdate{
				MessageId: &mid,
				From:      &from,
				Text:      &text,
				Timestamp: &ts,
			}); err != nil {
				return err
			}
		}
	}
}

func (s *GRPCServer) GetSessionState(ctx context.Context, req *pb.GetSessionStateRequest) (*pb.GetSessionStateResponse, error) {
	session, ok := s.sessions.GetSession(req.GetSessionId())
	if !ok {
		return nil, status.Error(codes.NotFound, "session not found")
	}

	var state string
	switch session.State() {
	case telegram.SessionStatePending:
		state = "PENDING"
	case telegram.SessionStateReady:
		state = "READY"
	case telegram.SessionStateClosed:
		state = "CLOSED"
	default:
		state = "UNKNOWN"
	}

	return &pb.GetSessionStateResponse{
		State: &state,
	}, nil
}

// mapTelegramError маппит ошибки Telegram/RPC в gRPC-статусы.
func mapTelegramError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return status.Error(codes.Canceled, err.Error())
	}

	// Пытаемся разобрать RPC-ошибку Telegram как tgerr.Error.
	if rpcErr, ok := tgerr.As(err); ok {
		// Ошибки, связанные с username (peer не существует или некорректен).
		// Используем префикс USERNAME_, чтобы не зависеть от точного списка
		// конкретных типов (USERNAME_INVALID, USERNAME_NOT_OCCUPIED и т.д.).
		if strings.HasPrefix(rpcErr.Type, "USERNAME_") {
			return status.Error(codes.NotFound, rpcErr.Error())
		}

		// Ошибки авторизации/прав.
		if rpcErr.IsCodeOneOf(401, 403) {
			return status.Error(codes.PermissionDenied, rpcErr.Error())
		}

		// Rate limit / FLOOD_WAIT — просим клиента замедлиться.
		if rpcErr.IsCode(420) || rpcErr.IsType(tgerr.ErrFloodWait) {
			return status.Error(codes.ResourceExhausted, rpcErr.Error())
		}

		// Временные проблемы на стороне Telegram (обычно 500).
		if rpcErr.IsCode(500) {
			return status.Error(codes.Unavailable, rpcErr.Error())
		}
	}

	return status.Errorf(codes.Internal, "telegram: %v", err)
}
