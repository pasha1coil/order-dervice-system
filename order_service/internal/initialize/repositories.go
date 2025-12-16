package initialize

import (
	"context"
	"order-service-system/order_service/internal/repository/bboltdb"
	"order-service-system/order_service/internal/repository/order_repository"

	"go.mongodb.org/mongo-driver/mongo"
)

type Repositories struct {
	OrderRepository *order_repository.OrderRepository
	BboltDBStore    *bboltdb.Store
}

type RepositoriesDeps struct {
	MongoDB *mongo.Database
}

func NewRepositories(ctx context.Context, deps RepositoriesDeps) (*Repositories, error) {
	if deps.MongoDB == nil {
		panic("mongo database must not be nil on <NewRepositories> of <initialize>")
	}
	orderRepo, err := order_repository.NewOrderRepository(ctx, order_repository.Deps{
		Collection: deps.MongoDB.Collection("order"),
	})
	if err != nil {
		return nil, err
	}

	bboltDBStore, err := bboltdb.Create()
	if err != nil {
		return nil, err
	}

	return &Repositories{
		OrderRepository: orderRepo,
		BboltDBStore:    bboltDBStore,
	}, nil
}
