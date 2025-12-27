package common

import (
	"sync"
)

// WorkerPool manages a pool of workers to process tasks concurrently
type WorkerPool struct {
	tasks chan func()
	wg    sync.WaitGroup
	quit  chan struct{}
}

// NewWorkerPool creates a new WorkerPool with the specified number of workers
func NewWorkerPool(numWorkers int) *WorkerPool {
	pool := &WorkerPool{
		tasks: make(chan func(), numWorkers*2), // Buffer size 2x workers
		quit:  make(chan struct{}),
	}

	for i := 0; i < numWorkers; i++ {
		pool.wg.Add(1)
		go pool.worker()
	}

	return pool
}

// worker processes tasks from the tasks channel
func (p *WorkerPool) worker() {
	defer p.wg.Done()
	for {
		select {
		case task := <-p.tasks:
			func() {
				defer func() {
					if r := recover(); r != nil {
						// Log panic but keep worker alive
						// In a real app we'd want access to the logger here
					}
				}()
				task()
			}()
		case <-p.quit:
			return
		}
	}
}

// Submit submits a task to the worker pool
func (p *WorkerPool) Submit(task func()) {
	select {
	case p.tasks <- task:
	case <-p.quit:
		// Pool is shutting down, don't accept new tasks
	}
}

// Stop stops the worker pool and waits for all workers to finish
func (p *WorkerPool) Stop() {
	close(p.quit)
	p.wg.Wait()
}
