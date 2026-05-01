package trixorm

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-redsync/redsync/v4"
	"github.com/go-redsync/redsync/v4/redis/goredis/v8"
	"github.com/hashicorp/go-multierror"
)

type lockerClient interface {
	Obtain(ctx context.Context, key string, options ...redsync.Option) (*redsync.Mutex, error)
}

type standardLockerClient struct {
	client *redsync.Redsync
}

func (l *standardLockerClient) Obtain(ctx context.Context, key string, options ...redsync.Option) (*redsync.Mutex, error) {
	mutex := l.client.NewMutex(key, options...)
	return mutex, mutex.LockContext(ctx)
}

type Locker struct {
	locker lockerClient
	r      *RedisCache
}

func (r *RedisCache) GetLocker() *Locker {
	if r.locker != nil {
		return r.locker
	}
	pool := goredis.NewPool(r.client)
	lockerClient := &standardLockerClient{client: redsync.New(pool)}
	r.locker = &Locker{locker: lockerClient, r: r}
	return r.locker
}

func (l *Locker) Obtain(key string, ttl time.Duration, waitTimeout time.Duration) (lock *Lock, obtained bool) {
	return l.ObtainContext(context.Background(), key, ttl, waitTimeout)
}

func (l *Locker) ObtainContext(ctx context.Context, key string, ttl time.Duration, waitTimeout time.Duration) (lock *Lock, obtained bool) {
	key = l.r.addNamespacePrefix(key)
	if ttl == 0 {
		panic(errors.New("ttl must be higher than zero"))
	}
	start := getNow(l.r.engine.hasRedisLogger)
	var mutex *redsync.Mutex
	var err error
	if waitTimeout == 0 {
		mutex, err = l.locker.Obtain(ctx, key, redsync.WithExpiry(ttl), redsync.WithTries(1))
	} else {
		minDelay := 50 * time.Millisecond
		tries := 10
		delay := time.Duration(waitTimeout.Nanoseconds() / int64(tries))
		if delay < minDelay {
			delay = minDelay
			tries = int(waitTimeout.Nanoseconds()/minDelay.Nanoseconds()) + 1
		}
		mutex, err = l.locker.Obtain(ctx, key, redsync.WithExpiry(ttl), redsync.WithTries(tries), redsync.WithRetryDelay(delay))
	}
	if err != nil {
		if isLockObtainFailure(err) {
			if l.r.engine.hasRedisLogger {
				message := fmt.Sprintf("LOCK OBTAIN %s TTL %s WAIT %s", key, ttl.String(), waitTimeout.String())
				l.fillLogFields("LOCK OBTAIN", message, start, true, nil)
			}
			return nil, false
		}
	}
	if l.r.engine.hasRedisLogger {
		message := fmt.Sprintf("LOCK OBTAIN %s TTL %s WAIT %s", key, ttl.String(), waitTimeout.String())
		l.fillLogFields("LOCK OBTAIN", message, start, false, nil)
	}
	checkError(err)
	lock = &Lock{lock: mutex, locker: l, key: key, ttl: ttl, has: true, engine: l.r.engine}
	return lock, true
}

type Lock struct {
	lock   *redsync.Mutex
	key    string
	ttl    time.Duration
	locker *Locker
	has    bool
	engine *Engine
}

func (l *Lock) Release() {
	if !l.has {
		return
	}
	l.has = false
	start := getNow(l.engine.hasRedisLogger)
	ok, err := l.lock.UnlockContext(context.Background())
	if errors.Is(err, redsync.ErrLockAlreadyExpired) || isLockContextError(err) {
		err = nil
	}
	if l.engine.hasRedisLogger {
		l.locker.fillLogFields("LOCK RELEASE", "LOCK RELEASE "+l.key, start, !ok, err)
	}
	checkError(err)
}

func (l *Lock) TTL() time.Duration {
	start := getNow(l.engine.hasRedisLogger)
	ttl := l.lock.Until().Sub(time.Now())
	if l.engine.hasRedisLogger {
		l.locker.fillLogFields("LOCK TTL", "LOCK TTL "+l.key, start, false, nil)
	}
	return ttl
}

func (l *Lock) Refresh(ctx context.Context) bool {
	if !l.has {
		return false
	}
	start := getNow(l.engine.hasRedisLogger)
	ok, err := l.lock.ExtendContext(ctx)
	if err != nil {
		if errors.Is(err, redsync.ErrExtendFailed) || isLockContextError(err) || isLockTaken(err) {
			ok = false
			err = nil
		}
	}
	if !ok {
		l.has = false
	}
	if l.engine.hasRedisLogger {
		message := fmt.Sprintf("LOCK REFRESH %s %s", l.key, l.ttl.String())
		l.locker.fillLogFields("LOCK REFRESH", message, start, !ok, err)
	}
	checkError(err)
	return ok
}

func (l *Locker) fillLogFields(operation, query string, start *time.Time, cacheMiss bool, err error) {
	fillLogFields(l.r.engine.queryLoggersRedis, l.r.config.GetCode(), sourceRedis, operation, query, start, cacheMiss, err)
}

func isLockContextError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	multiError := &multierror.Error{}
	if errors.As(err, &multiError) {
		for _, wrappedError := range multiError.Errors {
			if isLockContextError(wrappedError) {
				return true
			}
		}
	}
	return false
}

func isLockObtainFailure(err error) bool {
	if errors.Is(err, redsync.ErrFailed) || isLockContextError(err) {
		return true
	}
	return isLockTaken(err)
}

func isLockTaken(err error) bool {
	errTaken := &redsync.ErrTaken{}
	return errors.As(err, &errTaken)
}
