package telegram

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/tg"
)

// sendMessageReq — запрос на отправку сообщения из gRPC в горутину клиента.
type sendMessageReq struct {
	peer string
	text string
	resp chan sendMessageResp
}

type sendMessageResp struct {
	messageID int64
	err       error
}

// Session описывает одну Telegram-сессию (gotd-клиент, QR-авторизация, подписчики).
type Session struct {
	id    string
	appID int
	hash  string

	mu       sync.RWMutex
	state    SessionState
	qrCh     chan string // первый QR-токен для ответа CreateSession
	logoutCh chan struct{} // по сигналу вызываем auth.LogOut и выходим из run
	sendCh   chan sendMessageReq
	subs     map[chan *MessageUpdate]struct{}
	runDone  chan struct{}

	ctx    context.Context
	cancel context.CancelFunc
}

// MessageUpdate — входящее сообщение для подписчиков (дублирует pb.MessageUpdate по смыслу).
type MessageUpdate struct {
	MessageID int64
	From      string
	Text      string
	Timestamp int64
}

// getState возвращает текущее состояние сессии.
func (s *Session) getState() SessionState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// setState обновляет состояние сессии.
func (s *Session) setState(st SessionState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = st
}

// IsReady возвращает true, если сессия авторизована и готова к отправке/приёму.
func (s *Session) IsReady() bool {
	return s.getState() == SessionStateReady
}

// State возвращает текущее состояние сессии.
func (s *Session) State() SessionState {
	return s.getState()
}

// ID возвращает идентификатор сессии.
func (s *Session) ID() string {
	return s.id
}

// WaitQR блокируется до появления первого QR-токена или отмены контекста.
// Возвращает URL для QR-кода (tg://login?token=...).
func (s *Session) WaitQR(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case qr, ok := <-s.qrCh:
		if !ok {
			return "", fmt.Errorf("session closed before QR")
		}
		return qr, nil
	}
}

// NewSession создаёт сессию и запускает фоновую горутину (подключение, QR, цикл запросов).
func NewSession(id string, appID int, appHash string) *Session {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Session{
		id:       id,
		appID:    appID,
		hash:     appHash,
		state:    SessionStatePending,
		qrCh:     make(chan string, 1),
		logoutCh: make(chan struct{}, 1),
		sendCh:   make(chan sendMessageReq),
		subs:     make(map[chan *MessageUpdate]struct{}),
		runDone:  make(chan struct{}),
		ctx:      ctx,
		cancel:   cancel,
	}
	storage := &session.StorageMemory{}
	dispatcher := tg.NewUpdateDispatcher()
	dispatcher.OnNewMessage(s.handleNewMessage)
	go s.run(storage, &dispatcher)
	return s
}

// run запускает gotd-клиент в фоне: подключение, QR-авторизация, затем цикл запросов.
func (s *Session) run(storage telegram.SessionStorage, dispatcher *tg.UpdateDispatcher) {
	defer close(s.runDone)

	client := telegram.NewClient(s.appID, s.hash, telegram.Options{
		SessionStorage: storage,
		UpdateHandler:  dispatcher,
	})

	err := client.Run(s.ctx, func(ctx context.Context) error {
		// QR-авторизация: показываем токен через callback и ждём сканирования.
		loggedIn := qrlogin.OnLoginToken(dispatcher)
		show := func(ctx context.Context, token qrlogin.Token) error {
			url := token.URL()
			select {
			case s.qrCh <- url:
			default:
			}
			return nil
		}
		_, err := client.QR().Auth(ctx, loggedIn, show)
		if err != nil {
			return err
		}
		s.setState(SessionStateReady)

		// Цикл: обрабатываем запросы на отправку, logout или отмену контекста.
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-s.logoutCh:
				if _, err := client.API().AuthLogOut(ctx); err != nil {
					log.Printf("[session %s] AuthLogOut: %v", s.id, err)
				}
				return nil
			case req := <-s.sendCh:
				msgID, err := s.sendMessage(ctx, client, client.API(), req.peer, req.text)
				req.resp <- sendMessageResp{messageID: msgID, err: err}
			}
		}
	})
	if err != nil && s.ctx.Err() == nil {
		log.Printf("[session %s] client run error: %v", s.id, err)
	}
	s.setState(SessionStateClosed)
}

// sendMessage отправляет текстовое сообщение по peer (например @username).
func (s *Session) sendMessage(ctx context.Context, client *telegram.Client, api *tg.Client, peer, text string) (int64, error) {
	peer = strings.TrimPrefix(peer, "@")
	resolved, err := api.ContactsResolveUsername(ctx, peer)
	if err != nil {
		return 0, err
	}
	inputPeer := resolved.Peer
	// Преобразуем Peer в InputPeer для messages.SendMessage.
	var input tg.InputPeerClass
	switch p := inputPeer.(type) {
	case *tg.PeerUser:
		users := resolved.MapUsers().NotEmptyToMap()
		u, ok := users[p.UserID]
		if !ok || u == nil {
			return 0, fmt.Errorf("user %d not in resolved", p.UserID)
		}
		input = &tg.InputPeerUser{UserID: u.ID, AccessHash: u.AccessHash}
	case *tg.PeerChannel:
		chans := resolved.MapChats().ChannelToMap()
		ch, ok := chans[p.ChannelID]
		if !ok || ch == nil {
			return 0, fmt.Errorf("channel %d not in resolved", p.ChannelID)
		}
		input = &tg.InputPeerChannel{ChannelID: ch.ID, AccessHash: ch.AccessHash}
	case *tg.PeerChat:
		input = &tg.InputPeerChat{ChatID: p.ChatID}
	default:
		return 0, fmt.Errorf("unsupported peer type %T", inputPeer)
	}

	randomID, err := client.RandInt64()
	if err != nil {
		return 0, err
	}
	req := &tg.MessagesSendMessageRequest{
		Peer:      input,
		Message:   text,
		RandomID:  randomID,
		NoWebpage: true,
	}
	result, err := api.MessagesSendMessage(ctx, req)
	if err != nil {
		return 0, err
	}
	switch r := result.(type) {
	case *tg.Updates:
		if len(r.Updates) > 0 {
			if u, ok := r.Updates[0].(*tg.UpdateMessageID); ok {
				return int64(u.ID), nil
			}
		}
		return 0, nil
	case *tg.UpdateShortSentMessage:
		return int64(r.ID), nil
	default:
		return 0, fmt.Errorf("unexpected send result %T", result)
	}
}

// handleNewMessage вызывается из диспетчера апдейтов при новом сообщении.
func (s *Session) handleNewMessage(ctx context.Context, e tg.Entities, update *tg.UpdateNewMessage) error {
	msg, ok := update.Message.(*tg.Message)
	if !ok || msg.Message == "" {
		return nil
	}
	fromStr := ""
	if fromID, ok := msg.GetFromID(); ok {
		switch p := fromID.(type) {
		case *tg.PeerUser:
			if e.Users != nil {
				if u := e.Users[p.UserID]; u != nil {
					switch {
					case u.Username != "":
						// Для пользователей стараемся показывать @username.
						fromStr = "@" + u.Username
					case u.FirstName != "" || u.LastName != "":
						// Если username нет — показываем имя + фамилию вместо числового ID.
						fromStr = strings.TrimSpace(u.FirstName + " " + u.LastName)
					default:
						fromStr = fmt.Sprintf("%d", u.ID)
					}
				}
			}
		case *tg.PeerChannel:
			// Для каналов используем title, если есть.
			if e.Channels != nil {
				if ch := e.Channels[p.ChannelID]; ch != nil && ch.Title != "" {
					fromStr = ch.Title
				}
			}
		case *tg.PeerChat:
			// Для обычных чатов используем title.
			if e.Chats != nil {
				if ch := e.Chats[p.ChatID]; ch != nil && ch.Title != "" {
					fromStr = ch.Title
				}
			}
		}

		// Фолбэк: если username/title не нашли, показываем числовой ID отправителя.
		if fromStr == "" {
			switch p := fromID.(type) {
			case *tg.PeerUser:
				fromStr = fmt.Sprintf("%d", p.UserID)
			case *tg.PeerChannel:
				fromStr = fmt.Sprintf("%d", p.ChannelID)
			case *tg.PeerChat:
				fromStr = fmt.Sprintf("%d", p.ChatID)
			}
		}
	}
	mu := &MessageUpdate{
		MessageID: int64(msg.GetID()),
		From:      fromStr,
		Text:      msg.Message,
		Timestamp: int64(msg.GetDate()),
	}
	s.broadcast(mu)
	return nil
}

// broadcast рассылает обновление всем подписчикам (неблокирующе).
func (s *Session) broadcast(mu *MessageUpdate) {
	s.mu.RLock()
	for ch := range s.subs {
		select {
		case ch <- mu:
		default:
		}
	}
	s.mu.RUnlock()
}

// SendMessage отправляет текстовое сообщение указанному peer (например @username).
func (s *Session) SendMessage(ctx context.Context, peer, text string) (int64, error) {
	if s.getState() != SessionStateReady {
		return 0, fmt.Errorf("session not ready")
	}
	req := sendMessageReq{peer: peer, text: text, resp: make(chan sendMessageResp, 1)}
	select {
	case <-s.ctx.Done():
		return 0, s.ctx.Err()
	case s.sendCh <- req:
	}
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-s.ctx.Done():
		return 0, s.ctx.Err()
	case r := <-req.resp:
		return r.messageID, r.err
	}
}

// Subscribe добавляет канал подписчика на входящие сообщения.
// Возвращает функцию отписки.
func (s *Session) Subscribe() (ch <-chan *MessageUpdate, unsubscribe func()) {
	c := make(chan *MessageUpdate, 32)
	s.mu.Lock()
	if s.subs == nil {
		s.subs = make(map[chan *MessageUpdate]struct{})
	}
	s.subs[c] = struct{}{}
	s.mu.Unlock()
	once := sync.Once{}
	unsubscribe = func() {
		once.Do(func() {
			s.mu.Lock()
			delete(s.subs, c)
			s.mu.Unlock()
			close(c)
		})
	}
	return c, unsubscribe
}

// Stop останавливает клиент и освобождает ресурсы. При logout==true и авторизованной сессии
// вызывается auth.LogOut перед выходом (по требованию intro: «можно вызвать auth.logOut»).
// Контекст позволяет прервать ожидание корректного завершения, если оно зависло.
func (s *Session) Stop(ctx context.Context, logout bool) error {
	if logout && s.getState() == SessionStateReady {
		select {
		case s.logoutCh <- struct{}{}:
		default:
		}
	}

	select {
	case <-ctx.Done():
		s.cancel()
		return ctx.Err()
	case <-s.runDone:
	}

	s.cancel()
	s.mu.Lock()
	s.subs = nil
	s.mu.Unlock()
	s.setState(SessionStateClosed)
	return nil
}
