package app

import (
	"context"
	"log"
	"net"

	"google.golang.org/grpc"

	"pact-telegram/internal/config"
	"pact-telegram/internal/server"
	"pact-telegram/internal/telegram"
)

// App инкапсулирует зависимости и жизненный цикл gRPC-сервера и SessionManager.
type App struct {
	logger *log.Logger
	cfg    *config.Config

	sessionManager *telegram.SessionManager
	grpcServer     *grpc.Server
	lis            net.Listener
}

// New инициализирует приложение: создаёт SessionManager, listener и gRPC-сервер.
func New(logger *log.Logger, cfg *config.Config) (*App, error) {
	sessionManager := telegram.NewSessionManager(cfg.TelegramAppID, cfg.TelegramAppHash)

	lis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		return nil, err
	}

	grpcServer := grpc.NewServer()
	svc := server.NewGRPCServer(sessionManager)
	server.Register(grpcServer, svc)

	return &App{
		logger:         logger,
		cfg:            cfg,
		sessionManager: sessionManager,
		grpcServer:     grpcServer,
		lis:            lis,
	}, nil
}

// Run запускает gRPC-сервер и блокируется до завершения контекста или ошибки сервера.
// По завершении контекста выполняется корректное закрытие сессий и остановка gRPC-сервера.
func (a *App) Run(ctx context.Context) error {
	a.logger.Printf("gRPC server listening on %s\n", a.cfg.GRPCAddr)

	errCh := make(chan error, 1)
	go func() {
		errCh <- a.grpcServer.Serve(a.lis)
	}()

	select {
	case <-ctx.Done():
		// Грейсфул-шатдаун по сигналу/таймауту контекста.
		a.logger.Println("shutting down sessions")
		a.sessionManager.Shutdown()

		a.logger.Println("shutting down gRPC server")
		a.grpcServer.GracefulStop()

		return ctx.Err()
	case err := <-errCh:
		// Сервер завершился с ошибкой.
		return err
	}
}

