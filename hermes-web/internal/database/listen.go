package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// DocumentEvents is the channel the documents_notify trigger announces status changes on; the
// payload is the document id.
const DocumentEvents = "document_events"

// Listen delivers every notification on channel to onNotify until ctx is done. It holds one
// connection of its own (taken out of the pool, so a LISTENing connection is never reused for
// queries). When that connection fails it reconnects with backoff and calls onReconnect, since
// notifications sent while it was down are lost.
func Listen(ctx context.Context, pool *pgxpool.Pool, channel string, onNotify func(payload string), onReconnect func(), log *zap.Logger) {
	const minBackoff, maxBackoff = time.Second, 30 * time.Second
	backoff := minBackoff
	for first := true; ctx.Err() == nil; first = false {
		conn, err := listenConn(ctx, pool, channel)
		if err == nil {
			if !first {
				log.Info("listening again", zap.String("channel", channel))
				onReconnect()
			}
			backoff = minBackoff
			err = receive(ctx, conn, onNotify)
			conn.Close(context.Background())
		}
		if ctx.Err() != nil {
			return
		}
		log.Warn("listen connection lost, reconnecting", zap.String("channel", channel), zap.Error(err), zap.Duration("backoff", backoff))
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(2*backoff, maxBackoff)
	}
}

func listenConn(ctx context.Context, pool *pgxpool.Pool, channel string) (*pgx.Conn, error) {
	pc, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire: %w", err)
	}
	conn := pc.Hijack()
	if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{channel}.Sanitize()); err != nil {
		conn.Close(context.Background())
		return nil, fmt.Errorf("listen: %w", err)
	}
	return conn, nil
}

func receive(ctx context.Context, conn *pgx.Conn, onNotify func(string)) error {
	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			return err
		}
		onNotify(n.Payload)
	}
}
