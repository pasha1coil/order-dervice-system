package app

import (
	"context"
	"errors"
	"fmt"
	"order-service-system/common/closer"
	"order-service-system/common/mongo"
	"order-service-system/common/nats"
	"order-service-system/order_service/internal/initialize"
	"order-service-system/order_service/internal/server"
	"time"

	"go.uber.org/zap"
)

func Run(ctx context.Context, config initialize.Config, logger *zap.Logger) error {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("recovered from panic on <Run> of <app>", zap.Any("error", r))
		}
	}()

	logger.Info("service start on <Run> of <app>", zap.String("service", "order"), zap.Any("config", config))

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	shutdownGroup := closer.NewCloserGroup()

	mongoDB, err := mongo.Connect(ctx, &mongo.ConnectDeps{
		Configuration: &config.ExternalCfg.MongoConfig,
		Timeout:       10 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("failed connection to db: %w", err)
	}

	natsConn, err := nats.Connect(config.ExternalCfg.NatsConfig)
	if err != nil {
		return fmt.Errorf("failed connection to nats: %w", err)
	}

	repositories, err := initialize.NewRepositories(ctx, initialize.RepositoriesDeps{
		MongoDB: mongoDB,
	})
	if err != nil {
		return fmt.Errorf("failed initialize repositories: %w", err)
	}

	clients := initialize.NewClients(initialize.ClientsDeps{
		Logger:     logger,
		Conn:       natsConn,
		BboltStore: repositories.BboltDBStore,
	})

	services := initialize.NewServices(initialize.ServicesDeps{
		Logger:       logger,
		Clients:      clients,
		Repositories: repositories,
	})

	workers := initialize.NewWorkers(initialize.WorkersDeps{
		Logger:       logger,
		Clients:      clients,
		Repositories: repositories,
	})

	rpcControllers := initialize.NewRpcControllers(initialize.RpcControllersDeps{
		Services: services,
	})

	serverGRPC, err := server.NewGRPC(server.DepsGRPC{Logger: logger})
	if err != nil {
		return fmt.Errorf("failed initialize gRPC server: %w", err)
	}

	serverGRPC.Register(rpcControllers)
	go workers.RepublisherWC.Start(ctx, 3*time.Second)

	go func() {
		if err := serverGRPC.Run(config.GrpcURL); err != nil {
			logger.Error("gRPC server failed on <Run> of <app>", zap.Error(err))
			cancel()
		}
	}()

	shutdownGroup.Add(closer.CloserFunc(serverGRPC.Stop))
	shutdownGroup.Add(closer.CloserFunc(func(ctx context.Context) error {
		return natsConn.Drain()
	}))
	shutdownGroup.Add(closer.CloserFunc(mongoDB.Client().Disconnect))
	shutdownGroup.Add(closer.CloserFunc(func(ctx context.Context) error {
		return repositories.BboltDBStore.Close()
	}))

	<-ctx.Done()

	timeoutCtx, timeoutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer timeoutCancel()
	if err := shutdownGroup.Call(timeoutCtx); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			logger.Error("shutdown timed out on <Run> of <app>", zap.Error(err))
		} else {
			logger.Error("failed to shutdown services gracefully on <Run> of <app>", zap.Error(err))
		}
		return err
	}

	logger.Info("service stopped on <Run> of <app>", zap.String("service", "order"))
	return nil
}
