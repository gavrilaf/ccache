package ccache

import (
	"container/list"
	"hash/crc32"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

var (
	EvictionTick = 1 * time.Second
)

type Cache struct {
	shards  []*lruShard
	keys    *keysLRU
	size    int64
	maxSize int64

	touch chan string
	done  chan struct{}
}

const shardsCount = 16

func NewCache(maxSize int64) *Cache {
	shards := make([]*lruShard, shardsCount)
	for i := 0; i < shardsCount; i++ {
		shards[i] = newShard()
	}

	c := &Cache{
		shards:  shards,
		keys:    newKeys(),
		size:    0,
		maxSize: maxSize,
		touch:   make(chan string, 10),
		done:    make(chan struct{}),
	}

	c.start()

	return c
}

func (c *Cache) Set(key string, value interface{}, ttl time.Duration) bool {
	shard := hash(key) % shardsCount

	added := c.shards[shard].set(key, value, ttl)
	if added {
		atomic.AddInt64(&c.size, 1)
	}

	c.touch <- key

	return added
}

func (c *Cache) Get(key string) (interface{}, bool) {
	shard := hash(key) % shardsCount

	i, ok := c.shards[shard].get(key)
	if ok {
		c.touch <- key
	}

	// check on expire

	return i.value, ok
}

// private

func (c *Cache) del(key string) {
	shard := hash(key) % shardsCount
	c.shards[shard].del(key)

	atomic.AddInt64(&c.size, -1)
}

func (c *Cache) start() {
	go func() {
		for {
			select {
			case key := <-c.touch:
				c.keys.touch(key)
			case <-time.Tick(EvictionTick):
				c.evict()
			case <-c.done:
				break
			}
		}
	}()
}

func (c *Cache) evict() {
	for atomic.LoadInt64(&c.size) > c.maxSize {
		evicted := c.keys.evict()
		c.del(evicted)
	}
}

type item struct {
	value  interface{}
	expire time.Time
}

type lruShard struct {
	items map[string]item
	lock  sync.RWMutex
}

func newShard() *lruShard {
	return &lruShard{
		items: map[string]item{},
		lock:  sync.RWMutex{},
	}
}

func (s *lruShard) set(key string, value interface{}, ttl time.Duration) bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	_, ok := s.items[key]

	s.items[key] = item{
		value: value,
		//expire: time.Time{},
	}

	return !ok
}

func (s *lruShard) get(key string) (item, bool) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	i, ok := s.items[key]
	return i, ok
}

func (s *lruShard) del(key string) {
	s.lock.Lock()
	defer s.lock.Unlock()

	delete(s.items, key)
}

func hash(s string) int64 {
	bb := *(*[]byte)(unsafe.Pointer(&s))
	return int64(crc32.ChecksumIEEE(bb))
}

// key lru

func newKeys() *keysLRU {
	return &keysLRU{
		keys: map[string]*list.Element{},
		lst:  list.New(),
	}
}

type keysLRU struct {
	keys map[string]*list.Element
	lst  *list.List
}

func (k *keysLRU) touch(key string) {
	if e := k.keys[key]; e != nil {
		k.lst.MoveToBack(e)
	} else {
		k.keys[key] = k.lst.PushBack(key)
	}
}

func (k *keysLRU) evict() string {
	e := k.lst.Front()

	key := e.Value.(string)

	delete(k.keys, key)
	k.lst.Remove(e)

	return key
}
