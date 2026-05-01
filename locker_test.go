package trixorm

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocker(t *testing.T) {
	testLocker(t, "")
}

func TestLockerNamespace(t *testing.T) {
	testLocker(t, "test")
}

func TestLockerBlocksConcurrentAction(t *testing.T) {
	registry := &Registry{}
	registry.RegisterRedis("localhost:6382", "blocking_lock", 15)
	validatedRegistry, def, err := registry.Validate()
	require.NoError(t, err)
	defer def()
	engine := validatedRegistry.CreateEngine()
	engine.GetRedis().FlushDB()

	locker := engine.GetRedis().GetLocker()
	const holdDuration = 1000 * time.Millisecond

	firstAcquired := make(chan struct{})
	firstDone := make(chan struct{})
	secondDone := make(chan time.Duration, 1)
	errCh := make(chan error, 2)

	var mutex sync.Mutex
	actions := make([]string, 0, 3)
	recordAction := func(action string) {
		mutex.Lock()
		defer mutex.Unlock()
		actions = append(actions, action)
	}

	go func() {
		lock, hasLock := locker.ObtainContext(context.Background(), "blocking_key", time.Second*2, 0)
		if !hasLock {
			errCh <- errors.New("first goroutine did not obtain lock")
			close(firstAcquired)
			close(firstDone)
			return
		}

		close(firstAcquired)
		recordAction("first-start")
		time.Sleep(holdDuration)
		recordAction("first-finish")
		lock.Release()
		close(firstDone)
	}()

	select {
	case <-firstAcquired:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first goroutine to obtain lock")
	}
	select {
	case err := <-errCh:
		require.NoError(t, err)
	default:
	}

	secondStartedAt := time.Now()
	go func() {
		lock, hasLock := locker.ObtainContext(context.Background(), "blocking_key", time.Second*2, 2*time.Second)
		if !hasLock {
			errCh <- errors.New("second goroutine did not obtain lock after waiting")
			return
		}
		defer lock.Release()

		recordAction("second")
		secondDone <- time.Since(secondStartedAt)
	}()

	select {
	case elapsed := <-secondDone:
		t.Fatalf("second goroutine entered locked action too early after %s", elapsed)
	case <-time.After(holdDuration / 2):
	}

	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first goroutine to finish")
	}

	var elapsed time.Duration
	select {
	case elapsed = <-secondDone:
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for second goroutine to obtain lock")
	}

	assert.GreaterOrEqual(t, elapsed, holdDuration/2)

	mutex.Lock()
	defer mutex.Unlock()
	assert.Equal(t, []string{"first-start", "first-finish", "second"}, actions)
}

func testLocker(t *testing.T, namespace string) {
	registry := &Registry{}
	registry.RegisterRedis("localhost:6382", namespace, 15)
	validatedRegistry, def, err := registry.Validate()
	assert.Nil(t, err)
	defer def()
	engine := validatedRegistry.CreateEngine()
	engine.GetRedis().FlushDB()
	testLogger := &testLogHandler{}
	engine.RegisterQueryLogger(testLogger, false, true, false)

	l := engine.GetRedis().GetLocker()
	lock, has := l.ObtainContext(context.Background(), "test_key", time.Second, 0)
	assert.True(t, has)
	assert.NotNil(t, lock)
	has = lock.Refresh(context.Background())
	assert.True(t, has)

	_, has = l.ObtainContext(context.Background(), "test_key", time.Second, 100*time.Millisecond)
	assert.False(t, has)

	_, has = l.ObtainContext(context.Background(), "test_key", time.Second, 0)
	assert.False(t, has)

	left := lock.TTL()
	assert.LessOrEqual(t, left.Microseconds(), time.Second.Microseconds())

	lock.Release()
	lock.Release()
	has = lock.Refresh(context.Background())
	assert.False(t, has)

	assert.PanicsWithError(t, "ttl must be higher than zero", func() {
		_, _ = l.ObtainContext(context.Background(), "test_key", 0, time.Millisecond)
	})
}
