package server

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "pact-telegram/internal/pb/proto"
	"pact-telegram/internal/telegram"
)

func strPtr(s string) *string { return &s }

// fakeSessionManager — простая заглушка для юнит-тестов gRPC-слоя.
// Она позволяет эмулировать поведение SessionManager без реального подключения
// к Telegram.
type fakeSessionManager struct{}

func (f *fakeSessionManager) CreateSession(ctx context.Context) (string, string, error) {
	// В тесте мы заранее отменяем контекст, поэтому возвращаем его ошибку.
	return "", "", ctx.Err()
}

func (f *fakeSessionManager) DeleteSession(ctx context.Context, id string) error {
	return nil
}

func (f *fakeSessionManager) GetSession(id string) (*telegram.Session, bool) {
	return nil, false
}

func TestCreateSessionCanceledContext(t *testing.T) {
	m := &fakeSessionManager{}
	srv := &GRPCServer{sessions: m}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	cancel() // немедленно отменяем контекст

	_, err := srv.CreateSession(ctx, &pb.CreateSessionRequest{})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}

	st := status.Convert(err)
	if st.Code() != codes.Canceled {
		t.Fatalf("expected Canceled, got %v", st.Code())
	}
}

func TestDeleteSessionIdempotent(t *testing.T) {
	m := telegram.NewSessionManager(0, "")
	srv := NewGRPCServer(m)
	ctx := context.Background()

	_, err := srv.DeleteSession(ctx, &pb.DeleteSessionRequest{SessionId: strPtr("nonexistent")})
	if err != nil {
		t.Fatalf("DeleteSession should be idempotent: %v", err)
	}
}

func TestSendMessageNotFound(t *testing.T) {
	m := telegram.NewSessionManager(0, "")
	srv := NewGRPCServer(m)
	ctx := context.Background()

	_, err := srv.SendMessage(ctx, &pb.SendMessageRequest{
		SessionId: strPtr("nonexistent"),
		Peer:      strPtr("@someone"),
		Text:      strPtr("hi"),
	})
	if err == nil {
		t.Fatal("expected error for unknown session")
	}
	if st := status.Convert(err); st.Code() != codes.NotFound {
		t.Fatalf("expected NotFound, got %v", st.Code())
	}
}

func TestSubscribeMessagesNotFound(t *testing.T) {
	m := telegram.NewSessionManager(0, "")
	srv := NewGRPCServer(m)
	// Возвращаем NotFound до использования stream, поэтому передаём заглушку.
	err := srv.SubscribeMessages(&pb.SubscribeMessagesRequest{SessionId: strPtr("nonexistent")}, &dummySubscribeStream{})
	if err == nil {
		t.Fatal("expected error for unknown session")
	}
	if st := status.Convert(err); st.Code() != codes.NotFound {
		t.Fatalf("expected NotFound, got %v", st.Code())
	}
}

// dummySubscribeStream реализует TelegramService_SubscribeMessagesServer для тестов.
type dummySubscribeStream struct {
	pb.TelegramService_SubscribeMessagesServer
}

func (d *dummySubscribeStream) Send(*pb.MessageUpdate) error { return nil }
func (d *dummySubscribeStream) Context() context.Context     { return context.Background() }
