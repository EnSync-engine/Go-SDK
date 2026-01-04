package common

import (
	"log"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

type WorkerPool struct {
	tasks   chan func()
	wg      sync.WaitGroup
	quit    chan struct{}
	stopped int32
}

func NewWorkerPool(numWorkers int) *WorkerPool {
	pool := &WorkerPool{
		tasks: make(chan func(), numWorkers*2),
		quit:  make(chan struct{}),
	}

	for i := 0; i < numWorkers; i++ {
		pool.wg.Add(1)
		go pool.worker()
	}

	return pool
}

func (p *WorkerPool) worker() {
	defer p.wg.Done()
	for {
		select {
		case task := <-p.tasks:
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("[ERROR] Worker panic recovered: %v\nStack trace:\n%s",
							r, string(debug.Stack()))
					}
				}()
				task()
			}()
		case <-p.quit:
			return
		}
	}
}

func (p *WorkerPool) Submit(task func()) bool {
	if atomic.LoadInt32(&p.stopped) == 1 {
		return false
	}

	select {
	case <-p.quit:
		return false
	case p.tasks <- task:
		return true
	case <-time.After(100 * time.Millisecond):
		return false
	}
}

func (p *WorkerPool) Stop() {
	atomic.StoreInt32(&p.stopped, 1)
	close(p.quit)

	c := make(chan struct{})
	go func() {
		defer close(c)
		p.wg.Wait()
	}()

	select {
	case <-c:
		return
	case <-time.After(5 * time.Second):
		log.Println("[WARN] Worker pool stop timed out; some tasks may not have completed")
	}
}
