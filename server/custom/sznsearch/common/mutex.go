package common

import (
	"sync"
)

const (
	MutexMessageQueue = "message_queue"
)

// KeyedMutex provides named mutex locks for fine-grained concurrency control
type KeyedMutex struct {
	mutexes sync.Map
}

// NewKeyedMutex creates a new KeyedMutex
func NewKeyedMutex() *KeyedMutex {
	return &KeyedMutex{}
}

// WLock acquires a write lock for the given key
func (km *KeyedMutex) WLock(key string) {
	value, _ := km.mutexes.LoadOrStore(key, &sync.RWMutex{})
	mtx := value.(*sync.RWMutex)
	mtx.Lock()
}

// WUnlock releases a write lock for the given key
func (km *KeyedMutex) WUnlock(key string) {
	value, ok := km.mutexes.Load(key)
	if ok {
		mtx := value.(*sync.RWMutex)
		mtx.Unlock()
	}
}

// RLock acquires a read lock for the given key
func (km *KeyedMutex) RLock(key string) {
	value, _ := km.mutexes.LoadOrStore(key, &sync.RWMutex{})
	mtx := value.(*sync.RWMutex)
	mtx.RLock()
}

// RUnlock releases a read lock for the given key
func (km *KeyedMutex) RUnlock(key string) {
	value, ok := km.mutexes.Load(key)
	if ok {
		mtx := value.(*sync.RWMutex)
		mtx.RUnlock()
	}
}
