package main

/**
 * That's is an example KV store that can be used with the generic metadata key-value store.
 * Currently this one is not hooked in the aysfs by any mean. It's just for demonstration purposes.
 */
import (
	"github.com/Jumpscale/aysfs/metadata"
	"github.com/garyburd/redigo/redis"
	"time"
)

type redisStore struct {
	pool *redis.Pool
}

func newRedisStore(server string, password string) metadata.KeyValueStore {
	pool := &redis.Pool{
		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", server)
			if err != nil {
				return nil, err
			}

			if password != "" {
				if _, err := c.Do("AUTH", password); err != nil {
					c.Close()
					return nil, err
				}
			}

			return c, err
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}

	return &redisStore{
		pool: pool,
	}
}

func (r *redisStore) Set(key string, value []byte) error {
	db := r.pool.Get()
	defer db.Close()

	_, err := db.Do("SET", key, value)
	return err
}

func (r *redisStore) Get(key string) ([]byte, error) {
	db := r.pool.Get()
	defer db.Close()

	data, err := redis.Bytes(db.Do("GET", key))

	if err == redis.ErrNil {
		return nil, nil
	}

	return data, nil
}
