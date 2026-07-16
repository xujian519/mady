package agentcore

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestBrokerPublish(t *testing.T) {
	b := NewBroker[string]()
	defer b.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub := b.Subscribe(ctx)

	b.Publish("hello")

	select {
	case msg := <-sub:
		if msg != "hello" {
			t.Fatalf("expected 'hello', got %q", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestBrokerMultipleSubscribers(t *testing.T) {
	b := NewBroker[int]()
	defer b.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub1 := b.Subscribe(ctx)
	sub2 := b.Subscribe(ctx)
	sub3 := b.Subscribe(ctx)

	if n := b.SubscriberCount(); n != 3 {
		t.Fatalf("expected 3 subscribers, got %d", n)
	}

	b.Publish(42)

	for i, sub := range []<-chan int{sub1, sub2, sub3} {
		select {
		case msg := <-sub:
			if msg != 42 {
				t.Errorf("subscriber %d: expected 42, got %d", i+1, msg)
			}
		case <-time.After(time.Second):
			t.Errorf("subscriber %d: timed out", i+1)
		}
	}
}

func TestBrokerShutdown(t *testing.T) {
	b := NewBroker[string]()
	ctx := context.Background()
	sub := b.Subscribe(ctx)

	b.Shutdown()

	// Channel should be closed after shutdown.
	_, ok := <-sub
	if ok {
		t.Fatal("expected channel to be closed after shutdown")
	}

	// Publish after shutdown should not panic.
	b.Publish("after")
}

func TestBrokerContextCancel(t *testing.T) {
	b := NewBroker[string]()
	defer b.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	sub := b.Subscribe(ctx)

	// Cancel the context.
	cancel()

	// Channel should be closed after cancel — this synchronizes with the
	// cleanup goroutine. Once the channel is closed, the subscriber count
	// must be zero.
	_, ok := <-sub
	if ok {
		t.Fatal("expected channel to be closed after context cancel")
	}

	if n := b.SubscriberCount(); n != 0 {
		t.Fatalf("expected 0 subscribers after cancel, got %d", n)
	}
}

func TestBrokerDropCount(t *testing.T) {
	// Create a broker with a tiny buffer so we can trigger drops easily.
	b := NewBrokerWithOptions[int](1)
	defer b.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub := b.Subscribe(ctx)

	// Fill the buffer and trigger a drop.
	b.Publish(1) // fills buffer
	<-sub        // drain it

	// Now publish twice quickly to overflow.
	// The subscriber's buffer is 1, so the second publish should drop.
	b.Publish(2) // fills buffer
	b.Publish(3) // should be dropped (buffer full)

	if d := b.DropCount(); d != 1 {
		t.Fatalf("expected 1 drop, got %d", d)
	}
}

func TestBrokerPublishMustDeliver(t *testing.T) {
	b := NewBrokerWithOptions[int](1)
	b.SetMustDeliverTimeout(200 * time.Millisecond)
	defer b.Shutdown()

	ctx := context.Background()
	sub := b.Subscribe(ctx)

	// Fill the buffer with 1.
	b.Publish(1)

	// Start a goroutine that drains on signal, simulating async work completion.
	drain := make(chan struct{})
	drained := make(chan struct{})
	go func() {
		<-drain
		<-sub // drain the first message, freeing buffer space
		close(drained)
	}()

	// PublishMustDeliver should block until the goroutine drains the buffer.
	// Use a goroutine to unblock it after a brief scheduler yield.
	go func() {
		runtime.Gosched()
		close(drain)
	}()

	b.PublishMustDeliver(ctx, 2)
	<-drained

	// Verify we can still read message 2.
	select {
	case msg := <-sub:
		if msg != 2 {
			t.Fatalf("expected 2, got %d", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for second message")
	}

	if d := b.MustDeliverDropCount(); d != 0 {
		t.Fatalf("expected 0 must-deliver drops, got %d", d)
	}
}

func TestBrokerPublishMustDeliverTimeout(t *testing.T) {
	b := NewBrokerWithOptions[int](1)
	b.SetMustDeliverTimeout(10 * time.Millisecond)
	defer b.Shutdown()

	ctx := context.Background()
	sub := b.Subscribe(ctx)

	// Fill the buffer.
	b.Publish(1)

	// PublishMustDeliver should timeout because nobody drains the channel.
	b.PublishMustDeliver(ctx, 2)

	if d := b.MustDeliverDropCount(); d != 1 {
		t.Fatalf("expected 1 must-deliver drop, got %d", d)
	}

	// Drain to keep the channel from blocking.
	<-sub
}

func TestBrokerConcurrentPublish(t *testing.T) {
	b := NewBroker[int]()
	defer b.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())

	sub := b.Subscribe(ctx)

	const numGoroutines = 50
	const numPublishes = 100
	var pubWg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		pubWg.Add(1)
		go func() {
			defer pubWg.Done()
			for j := 0; j < numPublishes; j++ {
				b.Publish(j)
			}
		}()
	}

	// Collect results with a reader goroutine that tracks its own lifecycle.
	var count int
	var readWg sync.WaitGroup
	readWg.Add(1)
	go func() {
		defer readWg.Done()
		for msg := range sub {
			count++
			_ = msg
		}
	}()

	// Wait for all publishers to finish.
	pubWg.Wait()

	// Cancel the subscriber context to close the channel and stop the reader.
	cancel()

	// Wait for the reader to stop.
	readWg.Wait()

	expected := numGoroutines * numPublishes
	if count != expected {
		t.Logf("collected %d / %d events (drops: %d)", count, expected, b.DropCount())
	}
	if b.DropCount() > 0 {
		t.Logf("some events dropped under concurrent load: %d drops", b.DropCount())
	}
}

func TestBrokerSubscribeAfterShutdown(t *testing.T) {
	b := NewBroker[string]()
	b.Shutdown()

	ctx := context.Background()
	ch := b.Subscribe(ctx)

	// Channel should be closed immediately.
	_, ok := <-ch
	if ok {
		t.Fatal("expected closed channel when subscribing after shutdown")
	}
}

func TestBrokerNoSubscribers(t *testing.T) {
	b := NewBroker[int]()
	defer b.Shutdown()

	// Publishing with no subscribers should not panic.
	b.Publish(42)
	b.PublishMustDeliver(context.Background(), 42)
}
