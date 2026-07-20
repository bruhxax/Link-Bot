package cache

import (
	"sync"
	"time"
)

type Item struct {
	Value     int
	ExpiresAt time.Time
}

type Cache struct {
	data  map[int64]Item
	mutex sync.RWMutex
	ttl   time.Duration
}

func NewCache(ttl time.Duration) *Cache {
	c := &Cache{
		data: make(map[int64]Item),
		ttl:  ttl,
	}
	go c.cleanupExpired()
	return c
}

func (c *Cache) Set(key int64, value int) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.data[key] = Item{
		Value:     value,
		ExpiresAt: time.Now().Add(c.ttl),
	}
}

func (c *Cache) Get(key int64) (int, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	item, found := c.data[key]
	if !found || time.Now().After(item.ExpiresAt) {
		return 0, false
	}
	return item.Value, true
}

func (c *Cache) Delete(key int64) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	delete(c.data, key)
}

func (c *Cache) cleanupExpired() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		now := time.Now()
		c.mutex.Lock()
		for k, v := range c.data {
			if now.After(v.ExpiresAt) {
				delete(c.data, k)
			}
		}
		c.mutex.Unlock()
	}
}
