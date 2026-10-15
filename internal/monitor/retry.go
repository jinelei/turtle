package monitor

import (
	"context"
	"time"
)

const (
	defaultAttempts = 3
	defaultGap      = 300 * time.Millisecond
)

// Retry 对可重试的 DDC 错误（目前仅 Timeout）执行有限重试。
// 不可重试的错误立即返回；ctx 取消时立即返回。
func Retry(ctx context.Context, op func() error) error {
	var err error
	for i := 0; i < defaultAttempts; i++ {
		err = op()
		if err == nil {
			return nil
		}
		de, ok := err.(*Error)
		if !ok || !de.Retryable() {
			return err
		}
		if i == defaultAttempts-1 {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(defaultGap):
		}
	}
	return err
}
