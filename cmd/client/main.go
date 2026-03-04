package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/skip2/go-qrcode"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	pb "pact-telegram/internal/pb/proto"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:50051", "gRPC server address")
	sessionID := flag.String("session", "", "existing session id (optional, if empty a new session will be created)")
	subscribe := flag.Bool("subscribe", false, "receive incoming messages (stream); requires -session")
	peer := flag.String("peer", "", "destination peer, e.g. @username")
	text := flag.String("text", "hello from Go client", "message text")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, *addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		log.Fatalf("dial %s: %v", *addr, err)
	}
	defer conn.Close()

	client := pb.NewTelegramServiceClient(conn)

	if *subscribe {
		if *sessionID == "" {
			log.Fatal("для -subscribe нужен -session=<session_id> (уже авторизованная сессия)")
		}
		runSubscribe(context.Background(), client, *sessionID)
		return
	}

	if *peer == "" {
		log.Fatal("peer is required для отправки (e.g. -peer=@username)")
	}

	sessID := *sessionID
	if sessID == "" {
		sessID, err = createSessionQRFlow(ctx, client)
		if err != nil {
			log.Fatalf("create session: %v", err)
		}
	}

	// Отправляем сообщение (с повторными попытками при незавершённой авторизации).
	sendCtx, sendCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer sendCancel()

	var resp *pb.SendMessageResponse
	for {
		resp, err = client.SendMessage(sendCtx, &pb.SendMessageRequest{
			SessionId: &sessID,
			Peer:      peer,
			Text:      text,
		})
		if err != nil {
			if st, ok := status.FromError(err); ok {
				if st.Code() == codes.FailedPrecondition {
					log.Printf("Авторизация сессии ещё не завершена, ждём и пробуем снова...")
					select {
					case <-sendCtx.Done():
						log.Fatalf("SendMessage timeout while waiting for authorization: %v", sendCtx.Err())
					case <-time.After(3 * time.Second):
						continue
					}
				}
				log.Fatalf("SendMessage gRPC error: %s (%v)", st.Message(), st.Code())
			}
			log.Fatalf("SendMessage error: %v", err)
		}
		break
	}

	fmt.Printf("Message sent, id=%d\n", resp.GetMessageId())
}

// runSubscribe подписывается на входящие сообщения сессии и выводит их в консоль (Ctrl+C — выход).
func runSubscribe(ctx context.Context, client pb.TelegramServiceClient, sessionID string) {
	stream, err := client.SubscribeMessages(ctx, &pb.SubscribeMessagesRequest{SessionId: &sessionID})
	if err != nil {
		if st, ok := status.FromError(err); ok {
			log.Fatalf("SubscribeMessages: %s (%v)", st.Message(), st.Code())
		}
		log.Fatalf("SubscribeMessages: %v", err)
	}
	fmt.Printf("Подписка на входящие (session %s). Ctrl+C — выход.\n", sessionID)
	fmt.Println("Формат: id сообщения | отправитель | время | текст")
	fmt.Println()
	for {
		msg, err := stream.Recv()
		if err != nil {
			log.Printf("стрим завершён: %v", err)
			return
		}
		if msg == nil {
			continue
		}
		from := msg.GetFrom()
		if from == "" {
			from = "?"
		}
		ts := msg.GetTimestamp()
		timeStr := time.Unix(ts, 0).Format("02.01.2006 15:04:05")
		if ts == 0 {
			timeStr = "—"
		}
		fmt.Printf("  id=%d  from=%s  %s  │ %s\n", msg.GetMessageId(), from, timeStr, msg.GetText())
	}
}

func createSessionQRFlow(ctx context.Context, client pb.TelegramServiceClient) (string, error) {
	// Первое подключение к Telegram может занять до минуты.
	createCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	fmt.Println("Подключение к Telegram и получение QR… (подождите до ~1 мин)")
	resp, err := client.CreateSession(createCtx, &pb.CreateSessionRequest{})
	if err != nil {
		return "", err
	}

	qrURL := resp.GetQrCode()
	fmt.Println("New session created:")
	fmt.Printf("  session_id: %s\n", resp.GetSessionId())
	fmt.Printf("  qr_code   : %s\n", qrURL)
	if err := qrcode.WriteFile(qrURL, qrcode.Medium, 256, "qr.png"); err == nil {
		fmt.Println("  QR сохранён в qr.png — откройте файл и отсканируйте в Telegram (Settings → Devices → Scan QR).")
	} else {
		fmt.Println("  Сгенерируйте QR из ссылки выше и отсканируйте в Telegram.")
	}
	fmt.Print("Нажмите Enter после того, как авторизация завершится... ")

	reader := bufio.NewReader(os.Stdin)
	_, _ = reader.ReadBytes('\n')

	return resp.GetSessionId(), nil
}

