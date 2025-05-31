package throttler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrTooManyRequests = errors.New("too many requests")
	ErrClosed          = errors.New("throttler closed")
)

// Handler is the function you want to throttle.
type Handler func(context.Context) error

// Throttler limits the rate of executing handlers.
type Throttler interface {
	Throttle(h Handler) Handler
	Close()
}

func New(capacity, refill uint64, rate time.Duration) Throttler {
	t := &tokenBucket{
		capacity: capacity,
		tokens:   capacity,
		refill:   refill,
		rate:     rate,
		doneCh:   make(chan struct{}),
	}

	return t
}

type tokenBucket struct {
	capacity uint64        // Maximum token capacity
	tokens   uint64        // Current available tokens
	refill   uint64        // Number of tokens added per refill
	rate     time.Duration // Refill interval
	doneCh   chan struct{}
	closed   int32

	once sync.Once
	mu   sync.Mutex
}

func (t *tokenBucket) Throttle(h Handler) Handler {
	return func(ctx context.Context) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if atomic.LoadInt32(&t.closed) == 1 {
			return ErrClosed
		}

		t.once.Do(func() {
			go t.refillAlgorithm()
		})

		t.mu.Lock()

		if atomic.LoadInt32(&t.closed) == 1 {
			t.mu.Unlock()

			return ErrClosed
		}

		if t.tokens <= 0 {
			t.mu.Unlock()

			return ErrTooManyRequests
		}

		if ctx.Err() != nil {
			t.mu.Unlock()

			return ctx.Err()
		}

		t.tokens--

		t.mu.Unlock()

		return h(ctx)
	}
}

func (t *tokenBucket) refillAlgorithm() {
	ticker := time.NewTicker(t.rate)
	defer ticker.Stop()

	for {
		select {
		case <-t.doneCh:
			return
		case <-ticker.C:
			if atomic.LoadInt32(&t.closed) == 1 {
				return
			}

			t.mu.Lock()

			if atomic.LoadInt32(&t.closed) == 0 {
				t.tokens = min(t.tokens+t.refill, t.capacity)
			}
			t.mu.Unlock()

			if atomic.LoadInt32(&t.closed) == 1 {
				return
			}
		}
	}
}

func (t *tokenBucket) Close() {
	if !atomic.CompareAndSwapInt32(&t.closed, 0, 1) {
		return
	}

	close(t.doneCh)
}
