package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"ecommerce/backend/internal/db"
	"ecommerce/backend/internal/events"
	"ecommerce/backend/internal/store"
)

// Live updates (Server-Sent Events) and push subscriptions. Browsers never
// connect here directly: Next route handlers relay the streams, so the
// storefront key and session tokens stay on the server.

// sseKeepAlive is well under the idle timeouts of proxies and load balancers.
const sseKeepAlive = 25 * time.Second

type sseWriter struct {
	w  http.ResponseWriter
	rc *http.ResponseController
}

func startSSE(w http.ResponseWriter) *sseWriter {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache, no-transform")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no") // nginx: don't buffer the stream
	w.WriteHeader(http.StatusOK)
	sse := &sseWriter{w: w, rc: http.NewResponseController(w)}
	// An hour on Cloud Run at most anyway; EventSource reconnects by itself.
	_ = sse.rc.SetWriteDeadline(time.Time{})
	sse.comment("connected")
	return sse
}

func (s *sseWriter) event(name string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", name, data); err != nil {
		return err
	}
	return s.rc.Flush()
}

func (s *sseWriter) comment(text string) error {
	if _, err := fmt.Fprintf(s.w, ": %s\n\n", text); err != nil {
		return err
	}
	return s.rc.Flush()
}

// GET /v1/store/orders/{orderID}/events - the customer's order page. Sends
// the whole order on connect and again after every change.
func (s *Server) handleStorefrontOrderEvents(w http.ResponseWriter, r *http.Request) {
	branch, _ := branchFromContext(r.Context())
	orderID, err := uuid.Parse(chiURLParam(r, "orderID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "order not found")
		return
	}
	order, branchName, err := s.storefrontOrder(r, branch, orderID)
	if err != nil {
		s.writeOrderError(w, r, err)
		return
	}

	ch, cancel := s.events.Subscribe(func(e events.OrderEvent) bool { return e.OrderID == orderID })
	defer cancel()

	sse := startSSE(w)
	if err := sse.event("order", toOrderResponse(order, branchName)); err != nil {
		return
	}
	ping := time.NewTicker(sseKeepAlive)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			if sse.comment("ping") != nil {
				return
			}
		case <-ch:
			latest, err := s.orders.GetOrder(r.Context(), order.BranchID, orderID)
			if err != nil {
				if !errors.Is(err, r.Context().Err()) {
					s.logger.Error("order stream reload failed", "order_id", orderID, "error", err)
				}
				return
			}
			if sse.event("order", toOrderResponse(latest, branchName)) != nil {
				return
			}
		}
	}
}

type branchEventResponse struct {
	ID        string `json:"id"`
	Reference string `json:"reference"`
	Status    string `json:"status"`
	Op        string `json:"op"` // insert = a new order
}

// GET /v1/vendor/branches/{branchID}/events - the shop's screens. Each
// event says which order changed; the portal reloads what it shows.
func (s *Server) handleVendorBranchEvents(w http.ResponseWriter, r *http.Request) {
	branchID := vendorBranchID(r)
	ch, cancel := s.events.Subscribe(func(e events.OrderEvent) bool { return e.BranchID == branchID })
	defer cancel()

	sse := startSSE(w)
	ping := time.NewTicker(sseKeepAlive)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			if sse.comment("ping") != nil {
				return
			}
		case e := <-ch:
			if e.Op == "resync" {
				if sse.event("resync", struct{}{}) != nil {
					return
				}
				continue
			}
			// The shop never sees an unpaid order. Once it's paid, it reaches
			// the shop as a new order, so the vendor screen beeps then.
			if e.Status == "awaiting_payment" || e.OldStatus == "awaiting_payment" && e.Status != "pending" {
				continue // unpaid, or dropped unpaid
			}
			op := e.Op
			if e.OldStatus == "awaiting_payment" && e.Status == "pending" {
				op = "insert"
			}
			if sse.event("order", branchEventResponse{
				ID: e.OrderID.String(), Reference: e.Reference, Status: e.Status, Op: op,
			}) != nil {
				return
			}
		}
	}
}

// GET /v1/store/push/config - the key browsers subscribe with. "" = push
// isn't set up on this server.
func (s *Server) handlePushConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"public_key": s.push.PublicKey()})
}

// POST /v1/store/push-subscriptions
// {"subscription": {"endpoint", "keys": {"p256dh", "auth"}}, "order_id"?}
// Linked to the signed-in customer (all their orders) and/or one order.
func (s *Server) handleSavePushSubscription(w http.ResponseWriter, r *http.Request) {
	branch, _ := branchFromContext(r.Context())
	var req struct {
		Subscription struct {
			Endpoint string `json:"endpoint"`
			Keys     struct {
				P256dh string `json:"p256dh"`
				Auth   string `json:"auth"`
			} `json:"keys"`
		} `json:"subscription"`
		OrderID *uuid.UUID `json:"order_id"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sub := req.Subscription
	if u, err := url.Parse(sub.Endpoint); err != nil || u.Scheme != "https" || u.Host == "" ||
		sub.Keys.P256dh == "" || sub.Keys.Auth == "" || len(sub.Endpoint) > 2048 {
		writeError(w, http.StatusBadRequest, "subscription needs an https endpoint and both keys")
		return
	}

	customer, signedIn := customerFromContext(r.Context())
	var order *store.Order
	if req.OrderID != nil {
		o, _, err := s.storefrontOrder(r, branch, *req.OrderID)
		if err != nil {
			s.writeOrderError(w, r, err)
			return
		}
		order = &o
	}
	if order == nil && !signedIn {
		writeError(w, http.StatusBadRequest, "order_id is required unless signed in")
		return
	}

	params := db.UpsertPushSubscriptionParams{Endpoint: sub.Endpoint, P256dh: sub.Keys.P256dh, Auth: sub.Keys.Auth}
	if signedIn {
		params.CustomerAccountID = pgtype.UUID{Bytes: customer.ID, Valid: true}
	}
	saved, err := s.queries.UpsertPushSubscription(r.Context(), params)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if order != nil {
		if err := s.queries.LinkPushSubscriptionToOrder(r.Context(), db.LinkPushSubscriptionToOrderParams{
			OrderID: order.ID, SubscriptionID: saved.ID,
		}); err != nil {
			s.internalError(w, r, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /v1/store/push-subscriptions {"endpoint"} - the browser turned
// notifications off.
func (s *Server) handleDeletePushSubscription(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Endpoint string `json:"endpoint"`
	}
	if err := decodeJSON(w, r, &req); err != nil || req.Endpoint == "" {
		writeError(w, http.StatusBadRequest, "endpoint is required")
		return
	}
	if err := s.queries.DeletePushSubscription(r.Context(), req.Endpoint); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
