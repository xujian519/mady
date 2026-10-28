package cache

import (
	"container/list"
	"sync"
)

// LRU 是有界 LRU 容器：容量满时淘汰最久未访问的条目。并发安全。
type LRU[K comparable, V any] struct {
	mu       sync.Mutex
	capacity int
	order    *list.List // 前端为最近使用
	items    map[K]*list.Element
}

type lruEntry[K comparable, V any] struct {
	key   K
	value V
}

// NewLRU 创建容量为 capacity 的 LRU（capacity ≥ 1）。
func NewLRU[K comparable, V any](capacity int) *LRU[K, V] {
	if capacity < 1 {
		panic("cache: LRU capacity must be positive")
	}
	return &LRU[K, V]{
		capacity: capacity,
		order:    list.New(),
		items:    make(map[K]*list.Element, capacity),
	}
}

// Get 取值并把条目刷新为最近使用。ok=false 表示未命中。
func (c *LRU[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	element, ok := c.items[key]
	if !ok {
		var zero V
		return zero, false
	}
	c.order.MoveToFront(element)
	return element.Value.(*lruEntry[K, V]).value, true
}

// Set 写入并置为最近使用；容量满时淘汰最久未访问的条目。
func (c *LRU[K, V]) Set(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if element, ok := c.items[key]; ok {
		element.Value.(*lruEntry[K, V]).value = value
		c.order.MoveToFront(element)
		return
	}
	element := c.order.PushFront(&lruEntry[K, V]{key: key, value: value})
	c.items[key] = element
	if c.order.Len() > c.capacity {
		c.evictOldest()
	}
}

// Has 报告键是否存在（不改变使用序）。
func (c *LRU[K, V]) Has(key K) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.items[key]
	return ok
}

// Delete 删除键。
func (c *LRU[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if element, ok := c.items[key]; ok {
		c.order.Remove(element)
		delete(c.items, key)
	}
}

// Len 返回当前条目数。
func (c *LRU[K, V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

// Clear 清空全部条目。
func (c *LRU[K, V]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.order.Init()
	c.items = make(map[K]*list.Element, c.capacity)
}

func (c *LRU[K, V]) evictOldest() {
	oldest := c.order.Back()
	if oldest == nil {
		return
	}
	c.order.Remove(oldest)
	delete(c.items, oldest.Value.(*lruEntry[K, V]).key)
}
