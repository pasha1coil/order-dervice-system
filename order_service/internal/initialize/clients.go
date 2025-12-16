package initialize

import (
	"order-service-system/order_service/internal/clients/nats_client"
	"order-service-system/order_service/internal/repository/bboltdb"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

type Clients struct {
	NatsClient *nats_client.Client
}

type ClientsDeps struct {
	Logger     *zap.Logger
	Conn       *nats.Conn
	BboltStore *bboltdb.Store
}

func NewClients(deps ClientsDeps) *Clients {
	if deps.Logger == nil {
		panic("logger must not be nil on <NewClients> of <initialize>")
	}
	if deps.Conn == nil {
		panic("nats connection must not be nil on <NewClients> of <initialize>")
	}
	if deps.BboltStore == nil {
		panic("bbolt store must not be nil on <NewClients> of <initialize>")
	}
	return &Clients{
		NatsClient: nats_client.NewClient(nats_client.Deps{
			Logger:     deps.Logger,
			Conn:       deps.Conn,
			BboltStore: deps.BboltStore,
		}),
	}
}
