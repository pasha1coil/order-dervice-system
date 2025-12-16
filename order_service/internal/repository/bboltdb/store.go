package bboltdb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"order-service-system/order_service/internal/models"

	"go.etcd.io/bbolt"
)

const (
	orderCreatedBucket = "order_created"
	orderBboltPath     = "order_bbolt.db"
)

type Store struct {
	db *bbolt.DB
}

func Create() (*Store, error) {
	dir := filepath.Dir(orderBboltPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create dir for bbolt: %w", err)
	}

	db, err := bbolt.Open(orderBboltPath, 0o644, &bbolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open bbolt: %w", err)
	}

	if err := db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(orderCreatedBucket))
		return err
	}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create bucket: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) Save(event models.OrderCreatedEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(orderCreatedBucket))
		return b.Put([]byte(event.OrderID), data)
	})
}

func (s *Store) Range(fn func(key string, event models.OrderCreatedEvent) error) error {
	return s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(orderCreatedBucket))
		return b.ForEach(func(k, v []byte) error {
			var ev models.OrderCreatedEvent
			if err := json.Unmarshal(v, &ev); err != nil {
				return nil
			}
			return fn(string(k), ev)
		})
	})
}

func (s *Store) Delete(key string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(orderCreatedBucket))
		return b.Delete([]byte(key))
	})
}
