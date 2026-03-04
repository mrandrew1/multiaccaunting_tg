Разработать сервис на Go, который устанавливает и поддерживает несколько независимых
соединений с Telegram через библиотеку gotd.
Сервис должен позволять:
• динамически создавать и удалять соединения,
• отправлять текстовые сообщения,
• получать текстовые сообщения.
Взаимодействие с сервисом должно происходить через gRPC. Соединения должны быть
изолированы друг от друга. Проблемы с одним из соединений не должны влиять на работоспособность
сервиса (при этом допускается, что зависимости без багов и не паникуют).
Требования
• Тулчейн: Go 1.26 (последняя версия)
• Telegram клиент: github.com/gotd/td
• API: gRPC, Protocol Buffers (.proto)
• Хранение состояния в памяти (достаточно для тестового задания)
• Конфигурация: на ваш выбор, например, через переменные окружения или JSON файл
Создание соединения
При создании соединения сервис должен запускать процесс авторизации через QR и возвращать
данные для генерации QR кода. После этого пользователь:
1. Открывает Telegram на телефоне.
2. Переходит в Settings → Devices → Scan QR.
3. Сканирует QR-код.
Отправка сообщений
Сервис должен позволять отправлять текстовое сообщение через соответствующее Telegram-
соединение после завершения авторизации. Сообщения можно отправлять одновременно.
Получение сообщений
Сервис должен предоставлять возможность получения входящих текстовых сообщений для
конкретного соединения. Сообщение должно содержать идентификатор отправителя, текст,
время отправки и ID сообщения.
Удаление соединения
Операция удаления соединения должна останавливать клиент Telegram и освобождать связанные
ресурсы. Также можно вызвать метод auth.logOut, если авторизация была завершена.
Что необходимо предоставить
Исходный код в Git репозитории. README должен содержать инструкции по запуску, примером
вызова API (grpcurl или клиент на Go), описание архитектурных решений.

Пример .proto файла:

edition = "2023";
package pact.telegram;
service TelegramService {
rpc CreateSession(CreateSessionRequest) returns (CreateSessionResponse);
rpc DeleteSession(DeleteSessionRequest) returns (DeleteSessionResponse);
rpc SendMessage(SendMessageRequest) returns (SendMessageResponse);
rpc SubscribeMessages(SubscribeMessagesRequest) returns (stream MessageUpdate);
}
message CreateSessionRequest {}
message CreateSessionResponse {
string session_id = 1;
string qr_code = 2;
}
message DeleteSessionRequest {
string session_id = 1;
}
message DeleteSessionResponse {}
message SendMessageRequest {
string session_id = 1;
string peer = 2; // e.g. @durov
string text = 3;
}
message SendMessageResponse {
int64 message_id = 1;
}
message SubscribeMessagesRequest {
string session_id = 1;
}
message MessageUpdate {
int64 message_id = 1;
string from = 2;
string text = 3;
int64 timestamp = 4;
}