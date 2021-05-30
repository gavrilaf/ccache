package ccache_test

import (
	"math/rand"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/gavrilaf/ccache"
)

func TestCacheBasic(t *testing.T) {

	c := ccache.NewCache(10)

	t.Run("empty", func(t *testing.T) {
		_, ok := c.Get("k1")
		assert.False(t, ok)
	})

	t.Run("set/get simple value", func(t *testing.T) {
		added := c.Set("k1", 1, 0)
		assert.True(t, added)

		v, ok := c.Get("k1")
		assert.True(t, ok)
		assert.Equal(t, 1, v)
	})

	t.Run("add again", func(t *testing.T) {
		added := c.Set("k1", 1, 0)
		assert.False(t, added)
	})

	t.Run("replace value", func(t *testing.T) {
		added := c.Set("k1", 2, 0)
		assert.False(t, added)

		assertValue(t, c, "k1", 2)
	})

	t.Run("several values", func(t *testing.T) {
		c.Set("k2", 2, 0)
		c.Set("k3", 3, 0)

		assertValue(t, c, "k1", 2)
		assertValue(t, c, "k2", 2)
		assertValue(t, c, "k3", 3)
	})
}

func TestCacheEvixction(t *testing.T) {
	oldTick := ccache.EvictionTick
	ccache.EvictionTick = 100*time.Millisecond

	t.Run("eviction on set", func(t *testing.T) {
		c := ccache.NewCache(5)

		for i := 0; i < 7; i++ {
			c.Set(strconv.Itoa(i), i, 0)
		}

		time.Sleep(200*time.Millisecond)

		assertNoValue(t, c, "0")
		assertNoValue(t, c, "1")

		for i := 0; i < 5; i++ {
			assertValue(t, c, strconv.Itoa(i+2), i+2)
		}
	})

	t.Run("eviction when item was touched", func(t *testing.T) {
		c := ccache.NewCache(3)

		for i := 0; i < 3; i++ {
			c.Set(strconv.Itoa(i), i, 0)
		}

		assertValue(t, c, "0", 0)

		c.Set("3", 3, 0) // should evict key "1", because "0" was touched

		time.Sleep(200*time.Millisecond)

		assertNoValue(t, c, "1")

		assertValue(t, c, "0", 0)
		assertValue(t, c, "2", 2)
		assertValue(t, c, "3", 3)
	})

	ccache.EvictionTick = oldTick
}

func TestCacheConcurrency(t *testing.T) {
	c := ccache.NewCache(10)

	wg := sync.WaitGroup{}

	fn := func() {
		for i := 0; i < 10; i++ {
			c.Set(strconv.Itoa(i), i, 0)
		}
		wg.Done()
	}

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go fn()
	}

	wg.Wait()

	for i := 0; i < 10; i++ {
		assertValue(t, c, strconv.Itoa(i), i)
	}
}

func BenchmarkCache(b *testing.B) {
	c := ccache.NewCache(500)

	const valsCount = 1000
	vals := make([]int, valsCount)
	keys := make([]string, valsCount)
	for i := 0; i < valsCount; i++ {
		n := rand.Intn(1000)
		vals[i] = n
		keys[i] = strconv.Itoa(n)
	}

	b.Run("bench set", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			c.Set(keys[i % valsCount], vals[i % valsCount], 0)
		}
	})

	b.Run("bench get", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			c.Get(keys[i % valsCount])
		}
	})

	b.Run("set in parallel", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				c.Set(keys[i % valsCount], vals[i % valsCount], 0)
				i++
			}
		})
	})

	b.Run("get in parallel", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				c.Get(keys[i % valsCount])
				i++
			}
		})
	})
}



func assertValue(t *testing.T, c *ccache.Cache, k string, v interface{}) {
	vv, ok := c.Get(k)
	assert.True(t, ok)
	assert.Equal(t, v, vv)
}

func assertNoValue(t *testing.T, c *ccache.Cache, k string) {
	_, ok := c.Get(k)
	assert.False(t, ok)
}
