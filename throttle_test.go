package throttler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestThrottler_BasicOperation(t *testing.T) {
	throttler := New(3, 1, 100*time.Millisecond)
	defer throttler.Close()

	ctx := context.Background()
	callCount := int32(0)

	handler := func(ctx context.Context) error {
		atomic.AddInt32(&callCount, 1)
		return nil
	}

	throttledHandler := throttler.Throttle(handler)

	for i := 0; i < 3; i++ {
		if err := throttledHandler(ctx); err != nil {
			t.Errorf("call %d failed: %v", i+1, err)
		}
	}

	if err := throttledHandler(ctx); !errors.Is(err, ErrTooManyRequests) {
		t.Errorf("expected ErrTooManyRequests, got: %v", err)
	}

	if count := atomic.LoadInt32(&callCount); count != 3 {
		t.Errorf("expected handler to be called 3 times, got %d", count)
	}
}

func TestThrottler_TokenRefill(t *testing.T) {
	throttler := New(2, 1, 50*time.Millisecond)
	defer throttler.Close()

	ctx := context.Background()
	callCount := int32(0)

	handler := func(ctx context.Context) error {
		atomic.AddInt32(&callCount, 1)
		return nil
	}

	throttledHandler := throttler.Throttle(handler)

	for i := 0; i < 2; i++ {
		if err := throttledHandler(ctx); err != nil {
			t.Errorf("initial call %d failed: %v", i+1, err)
		}
	}

	if err := throttledHandler(ctx); !errors.Is(err, ErrTooManyRequests) {
		t.Errorf("expected ErrTooManyRequests, got: %v", err)
	}

	time.Sleep(60 * time.Millisecond)

	if err := throttledHandler(ctx); err != nil {
		t.Errorf("call after refill failed: %v", err)
	}

	if count := atomic.LoadInt32(&callCount); count != 3 {
		t.Errorf("expected handler to be called 3 times, got %d", count)
	}
}

func TestThrottler_MultipleRefills(t *testing.T) {
	throttler := New(5, 2, 30*time.Millisecond)
	defer throttler.Close()

	ctx := context.Background()
	handler := func(ctx context.Context) error { return nil }
	throttledHandler := throttler.Throttle(handler)

	for i := 0; i < 5; i++ {
		if err := throttledHandler(ctx); err != nil {
			t.Errorf("initial call %d failed: %v", i+1, err)
		}
	}

	time.Sleep(70 * time.Millisecond)

	successCount := 0
	for i := 0; i < 10; i++ {
		if err := throttledHandler(ctx); err == nil {
			successCount++
		}
	}

	if successCount < 2 {
		t.Errorf("expected at least 2 successful calls after refill, got %d", successCount)
	}
}

func TestThrottler_CapacityLimit(t *testing.T) {
	throttler := New(3, 5, 10*time.Millisecond)
	defer throttler.Close()

	ctx := context.Background()
	handler := func(ctx context.Context) error { return nil }
	throttledHandler := throttler.Throttle(handler)

	if err := throttledHandler(ctx); err != nil {
		t.Errorf("first call failed: %v", err)
	}

	time.Sleep(20 * time.Millisecond)

	successCount := 0
	for i := 0; i < 10; i++ {
		if err := throttledHandler(ctx); err == nil {
			successCount++
		}
	}

	if successCount != 3 {
		t.Errorf("expected exactly 3 successful calls (capacity limit), got %d", successCount)
	}
}

func TestThrottler_HandlerError(t *testing.T) {
	throttler := New(2, 1, 100*time.Millisecond)
	defer throttler.Close()

	ctx := context.Background()
	expectedErr := errors.New("handler error")

	handler := func(ctx context.Context) error {
		return expectedErr
	}

	throttledHandler := throttler.Throttle(handler)

	if err := throttledHandler(ctx); !errors.Is(err, expectedErr) {
		t.Errorf("expected handler error, got: %v", err)
	}

	if err := throttledHandler(ctx); !errors.Is(err, expectedErr) {
		t.Errorf("expected handler error, got: %v", err)
	}

	if err := throttledHandler(ctx); !errors.Is(err, ErrTooManyRequests) {
		t.Errorf("expected ErrTooManyRequests, got: %v", err)
	}
}

func TestThrottler_ContextCancellation(t *testing.T) {
	throttler := New(5, 1, 100*time.Millisecond)
	defer throttler.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	handler := func(ctx context.Context) error {
		t.Error("handler should not be called with cancelled context")
		return nil
	}

	throttledHandler := throttler.Throttle(handler)

	err := throttledHandler(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

func TestThrottler_ContextTimeout(t *testing.T) {
	throttler := New(5, 1, 100*time.Millisecond)
	defer throttler.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	time.Sleep(1 * time.Millisecond)

	handler := func(ctx context.Context) error {
		t.Error("handler should not be called with timed out context")
		return nil
	}

	throttledHandler := throttler.Throttle(handler)

	err := throttledHandler(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Expected context.DeadlineExceeded, got: %v", err)
	}
}

func TestThrottler_Close(t *testing.T) {
	throttler := New(5, 1, 100*time.Millisecond)

	ctx := context.Background()
	handler := func(ctx context.Context) error { return nil }
	throttledHandler := throttler.Throttle(handler)

	if err := throttledHandler(ctx); err != nil {
		t.Errorf("Call before close failed: %v", err)
	}

	throttler.Close()

	if err := throttledHandler(ctx); !errors.Is(err, ErrClosed) {
		t.Errorf("Expected ErrClosed, got: %v", err)
	}

	throttler.Close()
	throttler.Close()
}

func TestThrottler_ConcurrentAccess(t *testing.T) {
	throttler := New(100, 10, 10*time.Millisecond)
	defer throttler.Close()

	ctx := context.Background()
	var successCount int32
	var failureCount int32

	handler := func(ctx context.Context) error {
		time.Sleep(1 * time.Millisecond)
		return nil
	}

	throttledHandler := throttler.Throttle(handler)

	var wg sync.WaitGroup
	numGoroutines := 200

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := throttledHandler(ctx); err != nil {
				atomic.AddInt32(&failureCount, 1)
			} else {
				atomic.AddInt32(&successCount, 1)
			}
		}()
	}

	wg.Wait()

	totalCalls := atomic.LoadInt32(&successCount) + atomic.LoadInt32(&failureCount)
	if totalCalls != int32(numGoroutines) {
		t.Errorf("Expected %d total calls, got %d", numGoroutines, totalCalls)
	}

	if atomic.LoadInt32(&successCount) < 100 {
		t.Errorf("Expected at least 100 successful calls, got %d", atomic.LoadInt32(&successCount))
	}

	if atomic.LoadInt32(&failureCount) == 0 {
		t.Error("Expected some failures due to rate limiting")
	}
}

func TestThrottler_ConcurrentCloseAndThrottle(t *testing.T) {
	throttler := New(2, 1, 100*time.Millisecond)

	ctx := context.Background()
	handler := func(ctx context.Context) error {
		time.Sleep(10 * time.Millisecond)
		return nil
	}
	throttledHandler := throttler.Throttle(handler)

	var wg sync.WaitGroup
	var closedCount int32
	var startedCount int32

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				atomic.AddInt32(&startedCount, 1)
				if err := throttledHandler(ctx); errors.Is(err, ErrClosed) {
					atomic.AddInt32(&closedCount, 1)
					return
				}
				time.Sleep(500 * time.Microsecond)
			}
		}()
	}

	time.Sleep(5 * time.Millisecond)
	throttler.Close()

	wg.Wait()

	t.Logf("Started attempts: %d, Closed responses: %d",
		atomic.LoadInt32(&startedCount), atomic.LoadInt32(&closedCount))

	if atomic.LoadInt32(&closedCount) == 0 {
		t.Error("Expected some ErrClosed responses")
	}
}

func TestThrottler_CloseRejectsNewRequests(t *testing.T) {
	throttler := New(10, 1, 50*time.Millisecond)

	ctx := context.Background()
	handler := func(ctx context.Context) error { return nil }
	throttledHandler := throttler.Throttle(handler)

	_ = throttledHandler(ctx)

	throttler.Close()

	err := throttledHandler(ctx)
	if !errors.Is(err, ErrClosed) {
		t.Errorf("Expected ErrClosed, got %v", err)
	}

	for i := 0; i < 5; i++ {
		err := throttledHandler(ctx)
		if !errors.Is(err, ErrClosed) {
			t.Errorf("Request %d: Expected ErrClosed, got %v", i, err)
		}
	}
}

func TestThrottler_RefillAfterClose(t *testing.T) {
	throttler := New(1, 1, 10*time.Millisecond)

	ctx := context.Background()
	handler := func(ctx context.Context) error { return nil }
	throttledHandler := throttler.Throttle(handler)

	_ = throttledHandler(ctx)

	throttler.Close()

	time.Sleep(50 * time.Millisecond)

	if err := throttledHandler(ctx); !errors.Is(err, ErrClosed) {
		t.Errorf("Expected ErrClosed after close, got: %v", err)
	}
}

func BenchmarkThrottler_Sequential(b *testing.B) {
	throttler := New(uint64(b.N), 1, time.Second)
	defer throttler.Close()

	ctx := context.Background()
	handler := func(ctx context.Context) error { return nil }
	throttledHandler := throttler.Throttle(handler)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = throttledHandler(ctx)
	}
}

func BenchmarkThrottler_Concurrent(b *testing.B) {
	throttler := New(uint64(b.N), 1, time.Second)
	defer throttler.Close()

	ctx := context.Background()
	handler := func(ctx context.Context) error { return nil }
	throttledHandler := throttler.Throttle(handler)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = throttledHandler(ctx)
		}
	})
}
