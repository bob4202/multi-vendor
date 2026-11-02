// Package events turns Postgres notifications into in-process events. A
// trigger on orders runs pg_notify('order_events', ...) on every change, and
// each API instance keeps one LISTEN connection, so live screens hear about
// changes made by any instance, request or background job.
package events

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	orderChannel  = "order_events"
	outboxChannel = "notification_outbox"
)

// OrderEvent is one change to an order. Op is "insert" or "update", or
// "resync" after the hub reconnected and may have missed changes.
type OrderEvent struct {
	OrderID   uuid.UUID `json:"order_id"`
	BranchID  uuid.UUID `json:"branch_id"`
	Reference string    `json:"reference"`
	Status    string    `json:"status"`
	OldStatus string    `json:"old_status"` // empty on insert
	Op        string    `json:"op"`
}

type subscriber struct {
	ch     chan OrderEvent
	filter func(OrderEvent) bool
}

type Hub struct {
	logger     *slog.Logger
	mu         sync.Mutex
	subs       map[*subscriber]struct{}
	outboxWake chan struct{}
}

func NewHub(logger *slog.Logger) *Hub {
	return &Hub{
		logger:     logger,
		subs:       map[*subscriber]struct{}{},
		outboxWake: make(chan struct{}, 1),
	}
}

// Subscribe returns events matching filter until cancel is called. A slow
// subscriber misses events rather than blocking everyone else; each event
// only says "look again", so the next one catches it up.
func (h *Hub) Subscribe(filter func(OrderEvent) bool) (<-chan OrderEvent, func()) {
	s := &subscriber{ch: make(chan OrderEvent, 16), filter: filter}
	h.mu.Lock()
	h.subs[s] = struct{}{}
	h.mu.Unlock()
	var once sync.Once
	return s.ch, func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subs, s)
			h.mu.Unlock()
		})
	}
}

// OutboxWake fires when a notification was queued, so the sender doesn't
// have to wait for its next poll.
func (h *Hub) OutboxWake() <-chan struct{} {
	return h.outboxWake
}

func (h *Hub) publish(e OrderEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs {
		if e.Op != "resync" && !s.filter(e) {
			continue
		}
		select {
		case s.ch <- e:
		default:
		}
	}
}

func (h *Hub) wakeOutbox() {
	select {
	case h.outboxWake <- struct{}{}:
	default:
	}
}

// Run listens until ctx ends, reconnecting with backoff if the connection
// drops. After a reconnect every subscriber gets a "resync" event.
func (h *Hub) Run(ctx context.Context, pool *pgxpool.Pool) {
	backoff := time.Second
	first := true
	for ctx.Err() == nil {
		err := h.listen(ctx, pool, !first)
		if ctx.Err() != nil {
			return
		}
		first = false
		h.logger.Warn("event listener disconnected, retrying", "error", err, "in", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 30*time.Second)
	}
}

func (h *Hub) listen(ctx context.Context, pool *pgxpool.Pool, resync bool) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	// The connection is in LISTEN mode for good; never hand it back to the
	// pool for other queries.
	pgConn := conn.Hijack()
	defer pgConn.Close(context.Background())

	for _, ch := range []string{orderChannel, outboxChannel} {
		if _, err := pgConn.Exec(ctx, "LISTEN "+ch); err != nil {
			return err
		}
	}
	if resync {
		h.publish(OrderEvent{Op: "resync"})
		h.wakeOutbox()
	}

	for {
		n, err := pgConn.WaitForNotification(ctx)
		if err != nil {
			return err
		}
		switch n.Channel {
		case outboxChannel:
			h.wakeOutbox()
		case orderChannel:
			var e OrderEvent
			if err := json.Unmarshal([]byte(n.Payload), &e); err != nil {
				h.logger.Error("bad order event payload", "payload", n.Payload, "error", err)
				continue
			}
			h.publish(e)
		}
	}
}
