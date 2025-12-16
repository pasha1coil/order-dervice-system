package republisher

import (
	"context"
	"order-service-system/order_service/internal/clients/nats_client"
	"order-service-system/order_service/internal/models"
	"order-service-system/order_service/internal/repository/bboltdb"
	"time"

	"go.uber.org/zap"
)

type Republisher struct {
	logger     *zap.Logger
	bboltStore *bboltdb.Store
	natsClient *nats_client.Client
}

type Deps struct {
	Logger     *zap.Logger
	BboltStore *bboltdb.Store
	NatsClient *nats_client.Client
}

func NewRepublisher(deps Deps) *Republisher {
	if deps.Logger == nil {
		panic("logger must not be nil on <NewRepublisher> of <Republisher>")
	}
	if deps.BboltStore == nil {
		panic("bbolt store must not be nil on <NewRepublisher> of <Republisher>")
	}
	if deps.NatsClient == nil {
		panic("nats client must not be nil on <NewRepublisher> of <Republisher>")
	}
	return &Republisher{
		logger:     deps.Logger,
		bboltStore: deps.BboltStore,
		natsClient: deps.NatsClient,
	}
}

func (receiver *Republisher) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			receiver.republish()
		}
	}
}

func (receiver *Republisher) republish() {
	if err := receiver.bboltStore.Range(func(key string, event models.OrderCreatedEvent) error {
		if err := receiver.natsClient.PublishOrderCreated(event); err != nil {
			receiver.logger.Warn("retry publish failed on <republish> of <Republisher>",
				zap.String("order_id", event.OrderID),
				zap.Error(err),
			)
			return nil
		}
		if err := receiver.bboltStore.Delete(key); err != nil {
			receiver.logger.Warn("failed to delete bbolt entry after publish on <republish> of <Republisher>",
				zap.String("order_id", event.OrderID),
				zap.Error(err),
			)
		} else {
			receiver.logger.Info("republished event on <republish> of <Republisher>",
				zap.String("order_id", event.OrderID),
				zap.String("subject", "order.created"),
			)
		}
		return nil
	}); err != nil {
		receiver.logger.Warn("bbolt iteration failed on <republish> of <Republisher>", zap.Error(err))
	}
}
