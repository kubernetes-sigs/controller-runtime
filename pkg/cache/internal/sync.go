package internal

import (
	"context"
	"sync"
)

// anyOf returns a "done" channel that is closed when any of its input channels
// are closed or when the retuned cancel function is called, whichever comes first.
//
// The cancel function should always be called by the caller to ensure
// resources are properly released.
func anyOf(ch ...<-chan struct{}) (<-chan struct{}, context.CancelFunc) {
	var once sync.Once
	cancel := make(chan struct{})
	cancelFunc := func() {
		once.Do(func() {
			close(cancel)
		})
	}
	return anyInternal(append(ch, cancel)...), cancelFunc
}

func anyInternal(ch ...<-chan struct{}) <-chan struct{} {
	switch len(ch) {
	case 0:
		return nil
	case 1:
		return ch[0]
	}

	done := make(chan struct{})
	go func() {
		defer close(done)

		switch len(ch) {
		case 2:
			// This case saves a recursion + goroutine when there are exactly 2 channels.
			select {
			case <-ch[0]:
			case <-ch[1]:
			}
		default:
			// >=3 channels to merge
			select {
			case <-ch[0]:
			case <-ch[1]:
			case <-ch[2]:
			case <-anyInternal(append(ch[3:], done)...):
			}
		}
	}()

	return done
}

// mergeChan returns a channel that is closed when any of the input channels are signaled.
// The caller must call the returned CancelFunc to ensure no resources are leaked.
func mergeChan(a, b, c <-chan struct{}) (<-chan struct{}, context.CancelFunc) {
	var once sync.Once
	out := make(chan struct{})
	cancel := make(chan struct{})
	cancelFunc := func() {
		once.Do(func() {
			close(cancel)
		})
	}
	go func() {
		defer close(out)
		select {
		case <-a:
		case <-b:
		case <-c:
		case <-cancel:
		}
	}()

	return out, cancelFunc
}
