## Описание

Сервис на Go, который устанавливает и поддерживает **несколько независимых соединений с Telegram** через библиотеку `github.com/gotd/td` и предоставляет к ним доступ по **gRPC**.

Сервис позволяет:
- **создавать** и **удалять** Telegram‑сессии;
- **отправлять текстовые сообщения** через выбранную сессию;
- **подписываться на входящие сообщения** для конкретной сессии;
- **узнавать состояние сессии** (ожидает авторизации, готова, закрыта).

Контракт gRPC описан в `proto/telegram.proto` и соответствует заданию.

## Требования

- **Go**: 1.26.
- **Telegram client**: `github.com/gotd/td`.
- **gRPC / Protobuf**:
  - `google.golang.org/grpc`
  - `google.golang.org/protobuf`
- Для примеров ниже:
  - `grpcurl` для ручных вызовов API: [`https://github.com/fullstorydev/grpcurl`](https://github.com/fullstorydev/grpcurl).

## Конфигурация

Конфигурация читается из переменных окружения (см. `internal/config/config.go`):

- **`TELEGRAM_APP_ID`** — числовой `app_id` Telegram‑приложения.
- **`TELEGRAM_APP_HASH`** — строковый `app_hash` Telegram‑приложения.
- **`GRPC_ADDR`** — адрес gRPC‑сервера. По умолчанию `":50051"`.

### Получение Telegram app_id / app_hash

1. Перейти на `https://my.telegram.org`.
2. Войти под своим аккаунтом.
3. Открыть раздел **API development tools**.
4. Создать приложение и взять значения **App api_id** и **App api_hash**.
5. Перед запуском сервера выставить их в окружении:

```bash
export TELEGRAM_APP_ID=123456
export TELEGRAM_APP_HASH=your_app_hash_here
export GRPC_ADDR=":50051"   # опционально
```

## Сборка и запуск

Из корня репозитория:

```bash
go mod tidy          # при необходимости
go build -o bin/server ./cmd/server
```

Запуск сервера:

```bash
TELEGRAM_APP_ID=123456 \
TELEGRAM_APP_HASH=your_app_hash_here \
GRPC_ADDR=":50051" \
./bin/server
```

Либо без предварительной сборки:

```bash
TELEGRAM_APP_ID=123456 \
TELEGRAM_APP_HASH=your_app_hash_here \
GRPC_ADDR=":50051" \
go run ./cmd/server
```

По умолчанию сервер слушает `:50051` (см. `cmd/server/main.go` и `internal/config/config.go`).

## Запуск в Docker / docker-compose

### Сборка и запуск контейнера напрямую

```bash
docker build -t pact-telegram-server .

docker run --rm \
  -e TELEGRAM_APP_ID=123456 \
  -e TELEGRAM_APP_HASH=your_app_hash_here \
  -e GRPC_ADDR=":50051" \
  -p 50051:50051 \
  pact-telegram-server
```

В `Dockerfile` используется многослойная сборка: на первом этапе собирается бинарник, затем он
копируется в минимальный образ `alpine` с открытым портом `50051`.

### Запуск через docker-compose

В репозитории есть `docker-compose.yml`, который поднимает один сервис `server`:

```bash
export TELEGRAM_APP_ID=123456
export TELEGRAM_APP_HASH=your_app_hash_here

docker compose up --build
```

После этого gRPC‑сервер будет доступен на `localhost:50051`, и к нему можно обращаться через
`grpcurl`.

## Примеры вызова API (grpcurl)

Ниже примеры для локального сервера на `localhost:50051` и сервиса `pact.telegram.TelegramService`
из `proto/telegram.proto`.

### CreateSession

Создаёт новую Telegram‑сессию и запускает QR‑авторизацию. В ответе возвращается:

- `session_id` — идентификатор сессии;
- `qr_code` — URL вида `tg://login?token=...` для генерации QR‑кода.

```bash
grpcurl -plaintext \
  -d '{}' \
  localhost:50051 \
  pact.telegram.TelegramService/CreateSession
```

Пример ответа:

```json
{
  "sessionId": "c1b6d3d9-6e25-4e8c-b9e7-7a8e6c8f9a10",
  "qrCode": "tg://login?token=AAAA..."
}
```

QR‑код можно отобразить, например, через `qrencode`:

```bash
echo 'tg://login?token=AAAA...' | qrencode -t ANSIUTF8
```

Далее пользователь в Telegram:

1. Открывает приложение на телефоне.
2. Переходит в **Settings → Devices → Scan QR**.
3. Сканирует сгенерированный QR‑код.

После успешной авторизации сессия переходит в состояние **Ready** и готова к отправке/приёму сообщений.

Сразу после `CreateSession` сессия находится в состоянии **Pending** и станет **Ready** только после того, как пользователь отсканирует QR‑код в Telegram‑клиенте.

### SendMessage

Отправка текстового сообщения через уже авторизованную сессию:

```bash
grpcurl -plaintext \
  -d '{
    "sessionId": "c1b6d3d9-6e25-4e8c-b9e7-7a8e6c8f9a10",
    "peer": "@durov",
    "text": "Hello from pact-telegram!"
  }' \
  localhost:50051 \
  pact.telegram.TelegramService/SendMessage
```

Пример ответа:

```json
{
  "messageId": "123456789"
}
```

### SubscribeMessages (stream)

Подписка на входящие текстовые сообщения для конкретной сессии:

```bash
grpcurl -plaintext \
  -d '{
    "sessionId": "c1b6d3d9-6e25-4e8c-b9e7-7a8e6c8f9a10"
  }' \
  localhost:50051 \
  pact.telegram.TelegramService/SubscribeMessages
```

Сервер возвращает поток объектов `MessageUpdate`:

```json
{
  "messageId": "987654321",
  "from": "@someuser",
  "text": "Hi!",
  "timestamp": "1700000000"
}
```

### GetSessionState

Возвращает текущее состояние указанной сессии:

- `"PENDING"` — сессия создана, но ещё не авторизована (QR не отсканирован);
- `"READY"` — авторизация завершена, можно вызывать `SendMessage` и `SubscribeMessages`;
- `"CLOSED"` — сессия остановлена или удалена.

```bash
grpcurl -plaintext \
  -d '{
    "sessionId": "c1b6d3d9-6e25-4e8c-b9e7-7a8e6c8f9a10"
  }' \
  localhost:50051 \
  pact.telegram.TelegramService/GetSessionState
```

Пример ответа:

```json
{
  "state": "READY"
}
```

### DeleteSession

Удаление сессии и остановка клиента Telegram (с вызовом `auth.logOut` при завершённой авторизации):

```bash
grpcurl -plaintext \
  -d '{
    "sessionId": "c1b6d3d9-6e25-4e8c-b9e7-7a8e6c8f9a10"
  }' \
  localhost:50051 \
  pact.telegram.TelegramService/DeleteSession
```

Повторный вызов для того же `session_id` безопасен и не приводит к ошибке.

## Архитектура и основные компоненты

### Общая схема

- **gRPC‑сервер** (`cmd/server/main.go`, `internal/server`):
  - инициализирует конфигурацию и логгер;
  - создаёт `SessionManager`;
  - поднимает gRPC‑сервер и регистрирует реализацию `TelegramService`;
  - обрабатывает graceful shutdown (SIGINT/SIGTERM).

- **Менеджер сессий** (`internal/telegram/manager.go`):
  - хранит активные сессии в **памяти** (`map[session_id]*Session`);
  - создаёт новые сессии (`CreateSession`), возвращая `session_id` и URL для QR;
  - на удаление (`DeleteSession`) останавливает соответствующую `Session` и освобождает ресурсы.

- **Сессия Telegram** (`internal/telegram/session.go`):
  - одна логическая сессия Telegram (один аккаунт, одна авторизация);
  - хранит состояние (`Pending`, `Ready`, `Closed`);
  - запускает `gotd`‑клиент в отдельной горутине;
  - выполняет QR‑логин через `qrlogin.OnLoginToken` и отдаёт URL в канал `qrCh`;
  - после авторизации принимает запросы на отправку сообщений и обрабатывает входящие апдейты;
  - для входящих сообщений рассылает события подписчикам через метод `broadcast`.

- **Protocol Buffers / gRPC‑контракт** (`proto/telegram.proto`):
  - описывает сервис `TelegramService` и сообщения `CreateSessionRequest/Response`, `SendMessageRequest/Response`, `SubscribeMessagesRequest`, `MessageUpdate`, `DeleteSessionRequest/Response`;
  - Go‑код в пакете `internal/pb/proto` соответствует этому контракту и полностью генерируется из `proto/telegram.proto` при помощи `protoc` и плагинов `protoc-gen-go`, `protoc-gen-go-grpc`.

- **Конфигурация** (`internal/config/config.go`):
  - читает `GRPC_ADDR`, `TELEGRAM_APP_ID`, `TELEGRAM_APP_HASH` из окружения;
  - задаёт дефолтный адрес `:50051`.

### Изоляция сессий

- Каждая сессия (`Session`) имеет:
  - собственный контекст `context.Context` и `cancel`;
  - собственный `gotd`‑клиент и in‑memory‑хранилище (`session.StorageMemory`);
  - собственные каналы для запросов на отправку сообщений и подписчиков входящих сообщений.
- Ошибка или остановка одной сессии:
  - не влияет на другие элементы `SessionManager.sessions`;
  - не останавливает gRPC‑сервер и обработку других `session_id`.

### Хранение состояния

- В соответствии с заданием состояние хранится **в памяти процесса**:
  - карта `session_id → *Session` в `SessionManager`;
  - Telegram‑сессионные данные в `session.StorageMemory` из `github.com/gotd/td/session`.
- Перезапуск процесса приводит к потере всех сессий, что считается допустимым для тестового задания.

### Дальнейшие шаги

- **Отправка:** используйте тот же `session_id` для новых сообщений через grpcurl `SendMessage` или свой gRPC-клиент.
- **Приём:** подписка на входящие — grpcurl `SubscribeMessages` с вашим `session_id` (стрим в реальном времени) или клиент из `internal/pb/proto`.
- **Своё приложение:** подключайте gRPC-клиент из `internal/pb/proto` (или сгенерируйте по `proto/telegram.proto`) и вызывайте методы из своего кода.
- **Сессии:** `DeleteSession` для выхода; новая сессия — снова `CreateSession` (QR).

