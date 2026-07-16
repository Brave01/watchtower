package dedup

import (
	"crypto/md5"
	"fmt"
	"sync"
	"time"
)

// Deduplicator 去重器
type Deduplicator struct {
	mu       sync.RWMutex
	seen     map[string]time.Time
	ttl      time.Duration // 记录保留时间
	maxSize  int           // 最大记录数
}

// NewDeduplicator 创建去重器
func NewDeduplicator(ttl time.Duration, maxSize int) *Deduplicator {
	d := &Deduplicator{
		seen:    make(map[string]time.Time),
		ttl:     ttl,
		maxSize: maxSize,
	}

	// 定期清理过期记录
	go d.cleanup()

	return d
}

// IsDuplicate 检查是否重复，并返回是否为新记录
func (d *Deduplicator) IsDuplicate(key string) bool {
	d.mu.RLock()
	_, exists := d.seen[key]
	d.mu.RUnlock()
	return exists
}

// Mark 标记记录
func (d *Deduplicator) Mark(key string) {
	d.mu.Lock()
	d.seen[key] = time.Now()

	// 如果超出最大容量，删除最旧的
	if len(d.seen) > d.maxSize {
		d.evictOldest()
	}
	d.mu.Unlock()
}

// CheckAndMark 检查并标记（原子操作）
func (d *Deduplicator) CheckAndMark(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, exists := d.seen[key]; exists {
		return true
	}

	d.seen[key] = time.Now()

	if len(d.seen) > d.maxSize {
		d.evictOldest()
	}

	return false
}

// GenerateKey 根据日志内容生成去重键
func GenerateKey(rawJSON string) string {
	hash := md5.Sum([]byte(rawJSON))
	return fmt.Sprintf("%x", hash)
}

// Cleanup 清理过期记录
func (d *Deduplicator) cleanup() {
	ticker := time.NewTicker(d.ttl / 2)
	defer ticker.Stop()

	for range ticker.C {
		d.mu.Lock()
		now := time.Now()
		for key, added := range d.seen {
			if now.Sub(added) > d.ttl {
				delete(d.seen, key)
			}
		}
		d.mu.Unlock()
	}
}

func (d *Deduplicator) evictOldest() {
	oldestKey := ""
	oldestTime := time.Now()

	for key, added := range d.seen {
		if added.Before(oldestTime) {
			oldestTime = added
			oldestKey = key
		}
	}

	if oldestKey != "" {
		delete(d.seen, oldestKey)
	}
}

// Size 返回当前记录数
func (d *Deduplicator) Size() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.seen)
}
