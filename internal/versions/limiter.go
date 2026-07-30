package versions

import "context"

func withLimit(ctx context.Context, limit chan struct{}, fn func() error) error {
	if err := acquire(ctx, limit); err != nil {
		return err
	}
	defer release(limit)

	return fn()
}

func acquire(ctx context.Context, limit chan struct{}) error {
	select {
	case limit <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func release(limit chan struct{}) {
	<-limit
}
