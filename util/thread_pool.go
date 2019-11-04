package util

// ThreadPoolClosingError is a custom error class
type ThreadPoolClosingError struct{}

// Used to implement the error interface
func (err *ThreadPoolClosingError) Error() string {
	return "Thread pool is closing"
}

// TaskSubmissionFailedError is a custom error class
type TaskSubmissionFailedError struct{}

// Used to implement the error interface
func (err *TaskSubmissionFailedError) Error() string {
	return "Task submission failed"
}

const queueSize = 1024

// ThreadPool is a struct which holds the state of a thread pool
type ThreadPool struct {
	taskChannel chan func()
	isClosed    bool
}

// MakeThreadPool constructs a thread pool of workerCount workers
func MakeThreadPool(workerCount int) ThreadPool {
	pool := ThreadPool{}
	pool.taskChannel = make(chan func(), queueSize)
	pool.isClosed = false

	// run workers
	for ; workerCount > 0; workerCount-- {
		go func() {
			for f := range pool.taskChannel {
				f()
			}
		}()
	}

	return pool
}

// Submit submits a task to the thread pool. If all workers are busy and the queue is full, the operation returns false.
// The operation also returns false if the thread pool is closing
func (pool *ThreadPool) Submit(op func()) error {
	if !pool.isClosed {
		select {
		case pool.taskChannel <- op:
			return nil
		default:
			// Message submission failed
			return &TaskSubmissionFailedError{}
		}
	}
	return &ThreadPoolClosingError{}
}

// Terminate signals the termination of a thread pool. The work queue is cleared and all workers finish their tasks, then quit.
func (pool *ThreadPool) Terminate() {
	pool.isClosed = true
	close(pool.taskChannel)
}
