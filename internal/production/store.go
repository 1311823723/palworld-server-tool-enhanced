package production

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sort"

	"go.etcd.io/bbolt"
)

var (
	ordersBucket     = []byte("production_orders")
	metaBucket       = []byte("production_meta")
	secretKey        = []byte("bridge_secret")
	ErrOrderNotFound = errors.New("生产订单不存在")
)

type Store struct {
	db *bbolt.DB
}

func NewStore(db *bbolt.DB) (*Store, error) {
	store := &Store{db: db}
	if err := db.Update(func(tx *bbolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(ordersBucket); err != nil {
			return err
		}
		_, err := tx.CreateBucketIfNotExists(metaBucket)
		return err
	}); err != nil {
		return nil, err
	}
	return store, nil
}

func (store *Store) Secret() (string, error) {
	var secret string
	err := store.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(metaBucket)
		if value := bucket.Get(secretKey); len(value) > 0 {
			secret = string(value)
			return nil
		}
		buffer := make([]byte, 32)
		if _, err := rand.Read(buffer); err != nil {
			return err
		}
		secret = base64.RawURLEncoding.EncodeToString(buffer)
		return bucket.Put(secretKey, []byte(secret))
	})
	return secret, err
}

func (store *Store) PutOrder(order Order) error {
	data, err := json.Marshal(order)
	if err != nil {
		return err
	}
	return store.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(ordersBucket).Put([]byte(order.ID), data)
	})
}

func (store *Store) Order(id string) (Order, error) {
	var order Order
	err := store.db.View(func(tx *bbolt.Tx) error {
		value := tx.Bucket(ordersBucket).Get([]byte(id))
		if value == nil {
			return ErrOrderNotFound
		}
		return json.Unmarshal(value, &order)
	})
	return order, err
}

func (store *Store) ListOrders(limit int) ([]Order, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	orders := make([]Order, 0)
	err := store.db.View(func(tx *bbolt.Tx) error {
		return tx.Bucket(ordersBucket).ForEach(func(_, value []byte) error {
			var order Order
			if err := json.Unmarshal(value, &order); err != nil {
				return err
			}
			orders = append(orders, order)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(orders, func(i, j int) bool {
		return orders[i].CreatedAt.After(orders[j].CreatedAt)
	})
	if len(orders) > limit {
		orders = orders[:limit]
	}
	return orders, nil
}

func isOrderNotFound(err error) bool {
	return errors.Is(err, ErrOrderNotFound)
}
