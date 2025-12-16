package unit

import (
	"context"
	"errors"
	"order-service-system/order_service/internal/service/order_service"
	"testing"
	"time"

	"order-service-system/order_service/internal/models"
	"order-service-system/order_service/internal/pj_errors"
	orderpb "order-service-system/proto/order"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockOrderRepository struct {
	create       func(ctx context.Context, order models.Order) error
	get          func(ctx context.Context, orderID string) (models.Order, error)
	updateStatus func(ctx context.Context, orderID string, status string) (models.Order, error)
}

func (f *mockOrderRepository) Create(ctx context.Context, order models.Order) error {
	return f.create(ctx, order)
}

func (f *mockOrderRepository) Get(ctx context.Context, orderID string) (models.Order, error) {
	return f.get(ctx, orderID)
}

func (f *mockOrderRepository) UpdateStatus(ctx context.Context, orderID string, status string) (models.Order, error) {
	return f.updateStatus(ctx, orderID, status)
}

type mockNatsClient struct {
	publish func(event models.OrderCreatedEvent) error
}

func (f *mockNatsClient) PublishOrderCreated(event models.OrderCreatedEvent) error {
	return f.publish(event)
}

func newTestLogger(t *testing.T) *zap.Logger {
	t.Helper()
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	return logger
}

func TestNewOrderService_PanicsOnNilDeps(t *testing.T) {
	logger := newTestLogger(t)

	require.Panics(t, func() {
		order_service.NewOrderService(order_service.Deps{})
	})
	require.Panics(t, func() {
		order_service.NewOrderService(order_service.Deps{
			Logger:    logger,
			OrderRepo: &mockOrderRepository{},
		})
	})
	require.Panics(t, func() {
		order_service.NewOrderService(order_service.Deps{
			Logger:     logger,
			OrderRepo:  &mockOrderRepository{},
			NatsClient: nil,
		})
	})

	require.NotPanics(t, func() {
		_ = order_service.NewOrderService(order_service.Deps{
			Logger:     logger,
			OrderRepo:  &mockOrderRepository{},
			NatsClient: &mockNatsClient{},
		})
	})
}

func TestCreateOrder_ValidationErrors(t *testing.T) {
	logger := newTestLogger(t)

	svc := order_service.NewOrderService(order_service.Deps{
		Logger: logger,
		OrderRepo: &mockOrderRepository{
			create: func(ctx context.Context, order models.Order) error {
				t.Fatalf("Create should not be called on validation error")
				return nil
			},
		},
		NatsClient: &mockNatsClient{
			publish: func(event models.OrderCreatedEvent) error {
				t.Fatalf("PublishOrderCreated should not be called on validation error")
				return nil
			},
		},
	})

	tests := []struct {
		name string
		req  *orderpb.CreateOrderRequest
	}{
		{"nil request", nil},
		{"empty user", &orderpb.CreateOrderRequest{UserId: "", Items: []*orderpb.OrderItem{{ProductId: "p1", Quantity: 1, Price: 1}}}},
		{"no items", &orderpb.CreateOrderRequest{UserId: "u1"}},
		{"empty product_id", &orderpb.CreateOrderRequest{
			UserId: "u1",
			Items:  []*orderpb.OrderItem{{ProductId: "", Quantity: 1, Price: 1}},
		}},
		{"non-positive quantity", &orderpb.CreateOrderRequest{
			UserId: "u1",
			Items:  []*orderpb.OrderItem{{ProductId: "p1", Quantity: 0, Price: 1}},
		}},
		{"negative price", &orderpb.CreateOrderRequest{
			UserId: "u1",
			Items:  []*orderpb.OrderItem{{ProductId: "p1", Quantity: 1, Price: -1}},
		}},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.CreateOrder(ctx, tt.req)
			require.Error(t, err)
			st, ok := status.FromError(err)
			require.True(t, ok)
			require.Equal(t, codes.InvalidArgument, st.Code())
		})
	}
}

func TestCreateOrder_SuccessAndNatsErrorIsNonFatal(t *testing.T) {
	logger := newTestLogger(t)

	var createdOrder models.Order
	repoCalled := 0
	natsCalled := 0

	svc := order_service.NewOrderService(order_service.Deps{
		Logger: logger,
		OrderRepo: &mockOrderRepository{
			create: func(ctx context.Context, order models.Order) error {
				repoCalled++
				createdOrder = order
				return nil
			},
		},
		NatsClient: &mockNatsClient{
			publish: func(event models.OrderCreatedEvent) error {
				natsCalled++
				return errors.New("nats down")
			},
		},
	})

	req := &orderpb.CreateOrderRequest{
		UserId: "u1",
		Items: []*orderpb.OrderItem{
			{ProductId: "p1", Quantity: 2, Price: 10},
			{ProductId: "p2", Quantity: 1, Price: 5},
		},
	}

	ctx := context.Background()
	resp, err := svc.CreateOrder(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "u1", resp.UserId)
	require.Equal(t, orderpb.OrderStatus_PENDING, resp.Status)
	require.NotEmpty(t, resp.OrderId)

	require.Equal(t, 1, repoCalled)
	require.Equal(t, 1, natsCalled)
	require.Equal(t, createdOrder.UserID, resp.UserId)
}

func TestCreateOrder_RepoErrorReturnsInternal(t *testing.T) {
	logger := newTestLogger(t)

	svc := order_service.NewOrderService(order_service.Deps{
		Logger: logger,
		OrderRepo: &mockOrderRepository{
			create: func(ctx context.Context, order models.Order) error {
				return errors.New("db error")
			},
		},
		NatsClient: &mockNatsClient{
			publish: func(event models.OrderCreatedEvent) error {
				t.Fatalf("PublishOrderCreated should not be called when repo fails")
				return nil
			},
		},
	})

	req := &orderpb.CreateOrderRequest{
		UserId: "u1",
		Items: []*orderpb.OrderItem{
			{ProductId: "p1", Quantity: 1, Price: 10},
		},
	}

	ctx := context.Background()
	_, err := svc.CreateOrder(ctx, req)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.Internal, st.Code())
}

func TestGetOrder_ValidationAndErrorMapping(t *testing.T) {
	logger := newTestLogger(t)

	var requestID string
	svc := order_service.NewOrderService(order_service.Deps{
		Logger: logger,
		OrderRepo: &mockOrderRepository{
			get: func(ctx context.Context, orderID string) (models.Order, error) {
				requestID = orderID
				if orderID == "not_found" {
					return models.Order{}, pj_errors.ErrNotFound
				}
				if orderID == "db_err" {
					return models.Order{}, errors.New("db error")
				}
				return models.Order{
					OrderID:   orderID,
					UserID:    "u1",
					Status:    orderpb.OrderStatus_PENDING.String(),
					CreatedAt: time.Now(),
				}, nil
			},
		},
		NatsClient: &mockNatsClient{
			publish: func(event models.OrderCreatedEvent) error { return nil },
		},
	})

	ctx := context.Background()

	_, err := svc.GetOrder(ctx, "")
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.InvalidArgument, st.Code())

	_, err = svc.GetOrder(ctx, "not_found")
	require.Error(t, err)
	st, ok = status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.NotFound, st.Code())

	_, err = svc.GetOrder(ctx, "db_err")
	require.Error(t, err)
	st, ok = status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.Internal, st.Code())

	resp, err := svc.GetOrder(ctx, "order123")
	require.NoError(t, err)
	require.Equal(t, "order123", requestID)
	require.NotNil(t, resp)
	require.Equal(t, "order123", resp.OrderId)
}

func TestUpdateOrderStatus_ValidationAndErrorMapping(t *testing.T) {
	logger := newTestLogger(t)

	var getStatus string
	svc := order_service.NewOrderService(order_service.Deps{
		Logger: logger,
		OrderRepo: &mockOrderRepository{
			updateStatus: func(ctx context.Context, orderID string, status string) (models.Order, error) {
				getStatus = status
				if orderID == "not_found" {
					return models.Order{}, pj_errors.ErrNotFound
				}
				if orderID == "db_err" {
					return models.Order{}, errors.New("db error")
				}
				return models.Order{
					OrderID:   orderID,
					Status:    status,
					CreatedAt: time.Now(),
				}, nil
			},
		},
		NatsClient: &mockNatsClient{
			publish: func(event models.OrderCreatedEvent) error { return nil },
		},
	})

	ctx := context.Background()

	_, err := svc.UpdateOrderStatus(ctx, "", orderpb.OrderStatus_PAID)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.InvalidArgument, st.Code())

	_, err = svc.UpdateOrderStatus(ctx, "order1", orderpb.OrderStatus_ORDER_STATUS_UNSPECIFIED)
	require.Error(t, err)
	st, ok = status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.InvalidArgument, st.Code())

	_, err = svc.UpdateOrderStatus(ctx, "order1", orderpb.OrderStatus(999))
	require.Error(t, err)
	st, ok = status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.InvalidArgument, st.Code())

	_, err = svc.UpdateOrderStatus(ctx, "not_found", orderpb.OrderStatus_PAID)
	require.Error(t, err)
	st, ok = status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.NotFound, st.Code())

	_, err = svc.UpdateOrderStatus(ctx, "db_err", orderpb.OrderStatus_PAID)
	require.Error(t, err)
	st, ok = status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.Internal, st.Code())

	resp, err := svc.UpdateOrderStatus(ctx, "order1", orderpb.OrderStatus_PAID)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "PAID", getStatus)
	require.Equal(t, "order1", resp.OrderId)
	require.Equal(t, orderpb.OrderStatus_PAID, resp.Status)
}
