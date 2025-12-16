# Order Service System (Level 3)

Полный вариант тестового (уровень 3): три сервиса (order, billing, notification), MongoDB и NATS. Все собирается и стартует через `docker-compose`.

## Архитектура
- **order-service** — gRPC API. Хранит заказы в MongoDB, публикует `order.created` в NATS.
- **billing-service** — подписывается на `order.created`, имитирует оплату (1–2s), публикует `order.paid` или `order.failed`, вызывает `UpdateOrderStatus`.
- **notification-service** — подписывается на `order.paid`/`order.failed`, вызывает `UpdateOrderStatus` и логирует уведомление.
- **MongoDB** — основное хранилище заказов.
- **NATS** — шина данных.

Основные сабжекты:
- `order.created` — при создании заказа.
- `order.paid` / `order.failed` — результат оплаты.

## Запуск
Требуется Docker / Docker Compose.
```bash
docker-compose up --build
```
Порты по умолчанию:
- order-service gRPC: `localhost:50051`
- NATS: `localhost:4222`
- Mongo: `localhost:27017`

## Полный сценарий (grpcurl)
1) Создать заказ:
```bash
grpcurl -plaintext -d '{
  "userId": "u1",
  "items": [{"productId": "p1", "quantity": 2, "price": 10.5}]
}' localhost:50051 order.OrderService/CreateOrder
```
2) Получить заказ (статус PENDING):
```bash
grpcurl -plaintext -d '{"orderId": "<order_id>"}' localhost:50051 order.OrderService/GetOrder
```
3) Дождаться обработки биллингом: `order.paid` или `order.failed` публикуется в NATS, notification вызывает `UpdateOrderStatus`.
4) Проверить финальный статус:
```bash
grpcurl -plaintext -d '{"orderId": "<order_id>"}' localhost:50051 order.OrderService/GetOrder
```

## Переменные окружения (по умолчанию в docker-compose)
- `GRPC_URL` — адрес gRPC order-service (по умолчанию `:50051` внутри контейнера).
- `MONGO_URL`, `MONGO_DB_NAME` — подключение Mongo.
- `NATS_URL`, `NATS_CLIENT_NAME` — подключение NATS.
- `ORDER_SERVICE_HOST` — gRPC адрес order-service для billing/notification (по умолчанию `order-service:50051` в сети compose).
- `PAYMENT_SUCCESS_RATE` — вероятность успешной оплаты (0-1), дефолт 0.5.

## Тесты
- Юнит-тесты: `go test ./...`

## Поведение при ошибках
- Ошибка публикации `order.created` не фатальна: заказ сохраняется (PENDING). Событие кладётся в локальный bbolt, логируется WARN. Отдельный воркер периодически пытается перепубликовать и удаляет запись из bbolt при успехе.
- Ошибки оплаты/уведомлений логируются; сервисы продолжают работу. Ретраев для этих публикаций нет, но можно было сделать аналогично с bbolt.