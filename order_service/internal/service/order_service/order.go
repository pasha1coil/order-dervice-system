package order_service

import (
	"context"
	"errors"
	"order-service-system/order_service/internal/models"
	"order-service-system/order_service/internal/pj_errors"
	"order-service-system/order_service/internal/utils"
	orderpb "order-service-system/proto/order"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type OrderService struct {
	logger     *zap.Logger
	orderRepo  OrderRepository
	natsClient OrderEventsPublisher
}

type Deps struct {
	Logger     *zap.Logger
	OrderRepo  OrderRepository
	NatsClient OrderEventsPublisher
}

// только для unit тестов нужны
type OrderRepository interface {
	Create(ctx context.Context, order models.Order) error
	Get(ctx context.Context, orderID string) (models.Order, error)
	UpdateStatus(ctx context.Context, orderID string, status string) (models.Order, error)
}

type OrderEventsPublisher interface {
	PublishOrderCreated(event models.OrderCreatedEvent) error
}

func NewOrderService(deps Deps) *OrderService {
	if deps.Logger == nil {
		panic("logger must not be nil on <NewOrderService> of <OrderService>")
	}
	if deps.OrderRepo == nil {
		panic("order repo must not be nil on <NewOrderService> of <OrderService>")
	}
	if deps.NatsClient == nil {
		panic("nats client must not be nil on <NewOrderService> of <OrderService>")
	}
	return &OrderService{
		logger:     deps.Logger,
		orderRepo:  deps.OrderRepo,
		natsClient: deps.NatsClient,
	}
}

func (receiver *OrderService) CreateOrder(ctx context.Context, req *orderpb.CreateOrderRequest) (*orderpb.Order, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	if len(req.Items) == 0 {
		return nil, status.Error(codes.InvalidArgument, "items are required")
	}

	var items []models.OrderItem
	var total float64
	for _, item := range req.Items {
		if item.ProductId == "" {
			return nil, status.Error(codes.InvalidArgument, "product_id is required")
		}
		if item.Quantity <= 0 {
			return nil, status.Error(codes.InvalidArgument, "quantity must be positive")
		}
		if item.Price < 0 {
			return nil, status.Error(codes.InvalidArgument, "price must be non-negative")
		}
		items = append(items, models.OrderItem{
			ProductID: item.ProductId,
			Quantity:  item.Quantity,
			Price:     item.Price,
		})
		total += float64(item.Quantity) * item.Price
	}

	doc := models.Order{
		OrderID:     uuid.NewString(),
		UserID:      req.UserId,
		Items:       items,
		TotalAmount: total,
		Status:      orderpb.OrderStatus_PENDING.String(),
		CreatedAt:   time.Now(),
	}

	if err := receiver.orderRepo.Create(ctx, doc); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to persist order: %v", err)
	}

	if err := receiver.natsClient.PublishOrderCreated(models.OrderCreatedEvent{
		OrderID:     doc.OrderID,
		UserID:      doc.UserID,
		TotalAmount: doc.TotalAmount,
		CreatedAt:   doc.CreatedAt,
	}); err != nil {
		receiver.logger.Warn("failed to publish event on <CreateOrder> of <OrderService>",
			zap.String("subject", "order.created"),
			zap.Error(err),
		)
	}

	return utils.ConvertToProto(doc), nil
}

func (receiver *OrderService) GetOrder(ctx context.Context, orderID string) (*orderpb.Order, error) {
	if orderID == "" {
		return nil, status.Error(codes.InvalidArgument, "order_id is required")
	}
	doc, err := receiver.orderRepo.Get(ctx, orderID)
	if err != nil {
		if errors.Is(err, pj_errors.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "order not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get order: %v", err)
	}
	return utils.ConvertToProto(doc), nil
}

func (receiver *OrderService) UpdateOrderStatus(ctx context.Context, orderID string, newStatus orderpb.OrderStatus) (*orderpb.Order, error) {
	if orderID == "" {
		return nil, status.Error(codes.InvalidArgument, "order_id is required")
	}
	if newStatus == orderpb.OrderStatus_ORDER_STATUS_UNSPECIFIED {
		return nil, status.Error(codes.InvalidArgument, "status is required")
	}
	if _, ok := models.AllowedStatuses[newStatus]; !ok {
		return nil, status.Errorf(codes.InvalidArgument, "unsupported status %q", newStatus.String())
	}

	doc, err := receiver.orderRepo.UpdateStatus(ctx, orderID, newStatus.String())
	if err != nil {
		if errors.Is(err, pj_errors.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "order not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to update status: %v", err)
	}
	return utils.ConvertToProto(doc), nil
}
