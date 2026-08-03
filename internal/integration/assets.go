package integration

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// DefaultAssetTTL 是导出资产的默认有效期
const DefaultAssetTTL = 10 * time.Minute

type assetEntry struct {
	data      []byte
	mediaType string
	filename  string
	expiresAt time.Time
}

// AssetStore 是导出资产的内存暂存区，带惰性过期清理
type AssetStore struct {
	mu      sync.Mutex
	entries map[string]assetEntry
}

// NewAssetStore 创建资产暂存区
func NewAssetStore() *AssetStore {
	return &AssetStore{entries: make(map[string]assetEntry)}
}

// Put 存入资产，返回资产 ID 和过期时间；ttl 小于等于 0 时使用默认有效期
func (s *AssetStore) Put(data []byte, mediaType, filename string, ttl time.Duration) (id string, expiresAt time.Time) {
	if ttl <= 0 {
		ttl = DefaultAssetTTL
	}
	raw := make([]byte, 16)
	// crypto/rand 读取失败时退化为时间戳熵，仍可保证 ID 唯一性要求下的可用性
	if _, err := rand.Read(raw); err != nil {
		raw = []byte(time.Now().UTC().Format("20060102150405.000000000"))
	}
	id = hex.EncodeToString(raw)
	expiresAt = time.Now().Add(ttl)

	s.mu.Lock()
	s.entries[id] = assetEntry{data: data, mediaType: mediaType, filename: filename, expiresAt: expiresAt}
	s.mu.Unlock()
	return id, expiresAt
}

// Get 取出资产；过期资产会被惰性删除并视为不存在
func (s *AssetStore) Get(id string) (data []byte, mediaType, filename string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[id]
	if !ok {
		return nil, "", "", false
	}
	if time.Now().After(entry.expiresAt) {
		delete(s.entries, id)
		return nil, "", "", false
	}
	return entry.data, entry.mediaType, entry.filename, true
}
