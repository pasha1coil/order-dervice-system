package nats_client

import (
	"encoding/json"
	"fmt"
	"order-service-system/common/events"
	"order-service-system/order_service/internal/models"
	"order-service-system/order_service/internal/repository/bboltdb"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

type Client struct {
	conn       *nats.Conn
	logger     *zap.Logger
	bboltStore *bboltdb.Store
}

type Deps struct {
	Logger     *zap.Logger
	Conn       *nats.Conn
	BboltStore *bboltdb.Store
}

func NewClient(deps Deps) *Client {
	if deps.Logger == nil {
		panic("logger must not be nil on <NewClient> of <NatsClient>")
	}
	if deps.Conn == nil {
		panic("nats connection must not be nil on <NewClient> of <NatsClient>")
	}
	if deps.BboltStore == nil {
		panic("bbolt store must not be nil on <NewClient> of <NatsClient>")
	}
	return &Client{
		logger:     deps.Logger,
		conn:       deps.Conn,
		bboltStore: deps.BboltStore,
	}
}

func (receiver *Client) PublishOrderCreated(event models.OrderCreatedEvent) error {
	payload := events.OrderCreatedPayload{
		OrderID:     event.OrderID,
		UserID:      event.UserID,
		TotalAmount: event.TotalAmount,
		CreatedAt:   event.CreatedAt.Unix(),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	if err := receiver.conn.Publish("order.created", data); err != nil {
		if saveErr := receiver.bboltStore.Save(event); saveErr != nil {
			receiver.logger.Error("failed to persist event to bboltDB on <PublishOrderCreated> of <NatsClient>",
				zap.Error(saveErr),
				zap.String("order_id", event.OrderID),
			)
		}
		return fmt.Errorf("publish event: %w", err)
	}

	receiver.logger.Info("published event on <PublishOrderCreated> of <NatsClient>",
		zap.String("subject", "order.created"),
		zap.String("order_id", event.OrderID),
		zap.Time("created_at", event.CreatedAt),
	)
	return nil
}
