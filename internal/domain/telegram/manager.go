package telegram

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

// SessionState описывает состояние авторизации сессии.
type SessionState int

const (
	SessionStatePending SessionState = iota
	SessionStateReady
	SessionStateClosed
)

// SessionManager управляет жизненным циклом сессий.
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*Session

	appID   int
	appHash string
}

// NewSessionManager создаёт новый менеджер сессий.
func NewSessionManager(appID int, appHash string) *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
		appID:    appID,
		appHash:  appHash,
	}
}

// CreateSession создаёт новую сессию и запускает авторизацию по QR.
// Возвращает идентификатор сессии и строку для QR-кода (URL для отображения QR).
func (m *SessionManager) CreateSession(ctx context.Context) (sessionID string, qr string, err error) {
	m.mu.Lock()
	id := uuid.NewString()
	s := NewSession(id, m.appID, m.appHash)
	m.sessions[id] = s
	m.mu.Unlock()

	qr, err = s.WaitQR(ctx)
	if err != nil {
		_ = m.DeleteSession(ctx, id)
		return "", "", err
	}
	return id, qr, nil
}

// GetSession возвращает сессию по идентификатору.
func (m *SessionManager) GetSession(id string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	return s, ok
}

// DeleteSession останавливает и удаляет сессию (останавливает клиент, опционально auth.LogOut).
func (m *SessionManager) DeleteSession(ctx context.Context, id string) error {
	m.mu.Lock()
	s, ok := m.sessions[id]
	if !ok {
		m.mu.Unlock()
		return nil
	}
	delete(m.sessions, id)
	m.mu.Unlock()

	return s.Stop(ctx, true)
}

// Shutdown останавливает все активные сессии и очищает менеджер.
func (m *SessionManager) Shutdown() {
	m.mu.Lock()
	if len(m.sessions) == 0 {
		m.mu.Unlock()
		return
	}

	sessions := make([]*Session, 0, len(m.sessions))
	for id, s := range m.sessions {
		sessions = append(sessions, s)
		delete(m.sessions, id)
	}
	m.mu.Unlock()

	for _, s := range sessions {
		_ = s.Stop(context.Background(), true)
	}
}
