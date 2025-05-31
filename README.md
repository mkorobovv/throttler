# Throttler

![Go](https://img.shields.io/badge/Go-1.23+-blue.svg)
[![Go Reference](https://pkg.go.dev/badge/github.com/mkorobovv/throttler.svg)](https://pkg.go.dev/github.com/mkorobovv/throttler)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A simple token bucket rate limiter implementation in Go.

## Features

- **Token Bucket Algorithm**: Smooth rate limiting with burst capability
- **Thread-Safe**: Uses mutexes and atomic operations for concurrent access
- **Context Aware**: Properly handles context cancellations and timeouts
- **Efficient**: Minimal overhead when not throttling
- **Graceful Shutdown**: Clean resource cleanup on close

## Get started

```shell
go get github.com/mkorobovv/throttler
```

## Example

```go
package main

import (
	"context"
	"fmt"
	"time"
	
	"github.com/mkorobovv/throttler"
)

func main() {
	// Create throttler with:
	// - Capacity: 10 tokens (max burst)
	// - Refill: 2 tokens per interval
	// - Rate: 1 second interval
	t := throttler.New(10, 2, time.Second)
	defer t.Close()

	// Wrap your handler
	throttledHandler := t.Throttle(func(ctx context.Context) error {
		fmt.Println("Hello world!")
		return nil
	})

	// Simulate rapid requests
	for i := 0; i < 20; i++ {
		err := throttledHandler(context.Background())
		if err != nil {
			fmt.Println("Error:", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
```

## Contributing

1. Fork the repository
2. Create your feature branch (git checkout -b feature/fooBar)
3. Commit your changes (git commit -am 'Add some fooBar')
4. Push to the branch (git push origin feature/fooBar)
5. Create a new Pull Request