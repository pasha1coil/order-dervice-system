package initialize

import (
	"order-service-system/order_service/internal/workers/republisher"

	"go.uber.org/zap"
)

type Workers struct {
	RepublisherWC *republisher.Republisher
}

type WorkersDeps struct {
	Logger       *zap.Logger
	Clients      *Clients
	Repositories *Repositories
}

func NewWorkers(deps WorkersDeps) *Workers {
	if deps.Logger == nil {
		panic("logger must not be nil on <NewWorkers> of <initialize>")
	}
	if deps.Repositories == nil {
		panic("repositories must not be nil on <NewWorkers> of <initialize>")
	}
	if deps.Clients == nil {
		panic("clients must not be nil on <NewWorkers> of <initialize>")
	}
	return &Workers{
		RepublisherWC: republisher.NewRepublisher(republisher.Deps{
			Logger:     deps.Logger,
			BboltStore: deps.Repositories.BboltDBStore,
			NatsClient: deps.Clients.NatsClient,
		}),
	}
}
