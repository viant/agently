package integrate

import (
    "context"
    "time"
)

// SubscribeRunner starts a streaming subscription and returns a stop function and an error channel.
// Implementations should close errCh when the stream ends normally; send a non-nil error on auth/transport failures.
type SubscribeRunner func(ctx context.Context, token string) (stop func(), errCh <-chan error, err error)

// RunWithAuthReconnect runs a streaming subscription with bearer-first auth and performs a single
// reconnect attempt using a fresh token when the first error is received from the runner.
// The reconnect callback should rebuild any underlying transports, if necessary.
func RunWithAuthReconnect(ctx context.Context, tokenFn func(context.Context) (string, time.Time, error), reconnect func(context.Context) error, runner SubscribeRunner) error {
    // First start
    token, _, err := tokenFn(ctx)
    if err != nil {
        return err
    }
    stop, errCh, err := runner(ctx, token)
    if err != nil {
        return err
    }
    var retried bool
    for {
        select {
        case <-ctx.Done():
            if stop != nil { stop() }
            return ctx.Err()
        case rerr, ok := <-errCh:
            if !ok || rerr == nil {
                // Normal close
                return nil
            }
            if retried {
                if stop != nil { stop() }
                return rerr
            }
            retried = true
            if stop != nil { stop() }
            if reconnect != nil {
                if err := reconnect(ctx); err != nil {
                    return err
                }
            }
            // Fetch fresh token and restart
            token, _, err = tokenFn(ctx)
            if err != nil {
                return err
            }
            stop, errCh, err = runner(ctx, token)
            if err != nil {
                return err
            }
        }
    }
}

