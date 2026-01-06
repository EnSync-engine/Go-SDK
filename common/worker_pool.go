package common

import (
	"log"
	"sync"
	"sync/atomic"
)

type WorkerPool struct {
	tasks   chan func()
	wg      sync.WaitGroup
	quit    chan struct{}
	stopped int32
}

func NewWorkerPool(numWorkers int) *WorkerPool {
	pool := &WorkerPool{
		tasks: make(chan func(), numWorkers*workerTaskBufferMultiplier),
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
						log.Printf("[ERROR] Worker panic recovered: %v", r)
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
	default:
		return false
	}
}

func (p *WorkerPool) Stop() {
	atomic.StoreInt32(&p.stopped, 1)
	close(p.quit)
	p.wg.Wait()
}
