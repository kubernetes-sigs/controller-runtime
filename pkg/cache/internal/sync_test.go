package internal

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
)

var _ = Describe("anyOf", func() {
	// Generate contexts for different number of input channels
	for n := 0; n < 4; n++ {
		n := n
		Context(fmt.Sprintf("with %d channels", n), func() {
			var (
				channels []chan struct{}
				done     <-chan struct{}
				cancel   context.CancelFunc
			)
			BeforeEach(func() {
				channels = make([]chan struct{}, n)
				in := make([]<-chan struct{}, n)
				for i := 0; i < n; i++ {
					ch := make(chan struct{})
					channels[i] = ch
					in[i] = ch
				}
				done, cancel = anyOf(in...)
			})
			AfterEach(func() {
				cancel()
			})

			It("isn't closed initially", func() {
				select {
				case <-done:
					Fail("done was closed before cancel")
				case <-time.After(5 * time.Millisecond):
					// Ok.
				}
			})

			// Verify that done is closed when we call cancel explicitly.
			It("closes when cancelled", func() {
				cancel()
				select {
				case <-done:
					// Ok.
				case <-time.After(5 * time.Millisecond):
					Fail("timed out waiting for cancel")
				}
			})

			// Generate test cases for closing each individual channel.
			// Verify that done is closed in response.
			for i := 0; i < n; i++ {
				i := i
				It(fmt.Sprintf("closes when channel %d is closed", i), func() {
					close(channels[i])
					select {
					case <-done:
						// Ok.
					case <-time.After(5 * time.Millisecond):
						Fail("timed out waiting for cancel")
					}
				})
			}
		})
	}
})
