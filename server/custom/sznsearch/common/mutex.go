// Copyright (c) 2024-present Seznam.cz, a.s.
// All Rights Reserved.
//
// This file is part of custom extensions to Mattermost server
// developed by Seznam.cz, a.s. for internal use only.
//
// This code is a derivative work based on Mattermost server
// (https://github.com/mattermost/mattermost-server)
// Original Mattermost code is licensed under AGPL v3.0.
//
// This derivative work is used exclusively for internal purposes
// within Seznam.cz, a.s. and is not distributed to third parties.
// Therefore, the copyleft provisions of AGPL v3.0 do not apply
// as per the license's distribution requirements.

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
