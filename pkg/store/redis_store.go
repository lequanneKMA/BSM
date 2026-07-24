package store

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisStore struct {
	client *redis.Client
}

func NewRedisStore(addr string) (*RedisStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: "", // no password set by default
		DB:       0,  // use default DB
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	return &RedisStore{client: client}, nil
}

// AtomicReserveDriver executes SET driver:{driverID}:reservation bookingID NX EX ttl
func (r *RedisStore) AtomicReserveDriver(ctx context.Context, driverID, bookingID string, ttl time.Duration) (bool, error) {
	key := fmt.Sprintf("driver:%s:reservation", driverID)
	success, err := r.client.SetNX(ctx, key, bookingID, ttl).Result()
	if err != nil {
		return false, err
	}
	return success, nil
}

// ReleaseDriverReservation removes the reservation lock
func (r *RedisStore) ReleaseDriverReservation(ctx context.Context, driverID string) error {
	key := fmt.Sprintf("driver:%s:reservation", driverID)
	return r.client.Del(ctx, key).Err()
}

// GetDriverReservation returns current booking ID reserved for driver
func (r *RedisStore) GetDriverReservation(ctx context.Context, driverID string) (string, error) {
	key := fmt.Sprintf("driver:%s:reservation", driverID)
	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

// SetCustomerDriverCooldown locks the (customerID, driverID) pair for a specified duration (e.g. 15-30m)
func (r *RedisStore) SetCustomerDriverCooldown(ctx context.Context, customerID, driverID string, ttl time.Duration) error {
	if customerID == "" || driverID == "" {
		return nil
	}
	key := fmt.Sprintf("cooldown:cust:%s:drv:%s", customerID, driverID)
	return r.client.Set(ctx, key, "CANCELLED", ttl).Err()
}

// GetCustomerCooldownDrivers returns a list of driver IDs currently in cooldown for the given customerID
func (r *RedisStore) GetCustomerCooldownDrivers(ctx context.Context, customerID string) []string {
	if customerID == "" {
		return nil
	}
	pattern := fmt.Sprintf("cooldown:cust:%s:drv:*", customerID)
	keys, err := r.client.Keys(ctx, pattern).Result()
	if err != nil || len(keys) == 0 {
		return nil
	}

	var driverIDs []string
	prefix := fmt.Sprintf("cooldown:cust:%s:drv:", customerID)
	for _, key := range keys {
		if len(key) > len(prefix) {
			driverIDs = append(driverIDs, key[len(prefix):])
		}
	}
	return driverIDs
}
