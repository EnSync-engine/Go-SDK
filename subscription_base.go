package ensync

import (
	"sync"
)

type baseSubscription struct {
	handlers []EventHandler
	mu       sync.RWMutex
}

func (b *baseSubscription) AddHandler(handler EventHandler) func() {
	b.mu.Lock()
	b.handlers = append(b.handlers, handler)
	b.mu.Unlock()
	
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		
		for i, h := range b.handlers {
			if &h == &handler {
				b.handlers = append(b.handlers[:i], b.handlers[i+1:]...)
				break
			}
		}
	}
}

func (b *baseSubscription) GetHandlers() []EventHandler {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	handlers := make([]EventHandler, len(b.handlers))
	copy(handlers, b.handlers)
	return handlers
}
