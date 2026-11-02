package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"ecommerce/backend/internal/db"
)

// Substitutions: while picking, staff can mark a line unavailable and either
// remove it or offer a similar product. The customer must accept an offer;
// no answer in time removes the line. The customer never pays more than
// they ordered: a substitute's line costs min(original line, substitute).

var (
	ErrItemNotFound            = errors.New("item not found on this order")
	ErrOrderNotChangeable      = errors.New("items can only change while the order is accepted or being picked")
	ErrItemAlreadyChanged      = errors.New("this item has already been removed or substituted")
	ErrSubstitutionsNotAllowed = errors.New("the customer asked for no substitutions - remove the item instead")
	ErrSubstituteUnavailable   = errors.New("that product isn't available at this branch")
	ErrSubstituteSameProduct   = errors.New("choose a different product as the substitute")
	ErrInvalidQuantity         = errors.New("quantity must be between 1 and 99")
	ErrSubstitutionNotFound    = errors.New("substitution not found")
	ErrSubstitutionClosed      = errors.New("this substitution has already been answered or has expired")
	ErrNoLiveSubstitution      = errors.New("this item has no substitution waiting for an answer")
)

// allItemsGoneReason is shown to the customer when the last line goes.
const allItemsGoneReason = "None of the items were available"

type SubstituteInput struct {
	ProductID uuid.UUID
	Quantity  int
}

type MarkUnavailableInput struct {
	BranchID    uuid.UUID
	OrderID     uuid.UUID
	ItemID      uuid.UUID
	ActorUserID uuid.UUID
	// Substitute is the product offered instead; nil removes the line.
	Substitute *SubstituteInput
}

// MarkItemUnavailable removes a line, or offers a substitute for it.
func (s *Service) MarkItemUnavailable(ctx context.Context, in MarkUnavailableInput) (Order, error) {
	if in.Substitute != nil && (in.Substitute.Quantity < 1 || in.Substitute.Quantity > 99) {
		return Order{}, ErrInvalidQuantity
	}
	return s.inOrderTx(ctx, in.BranchID, in.OrderID, func(q *db.Queries, order db.Order) error {
		if order.Status != "accepted" && order.Status != "picking" {
			return ErrOrderNotChangeable
		}
		item, err := q.GetOrderItemForOrder(ctx, db.GetOrderItemForOrderParams{ID: in.ItemID, OrderID: order.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrItemNotFound
		}
		if err != nil {
			return err
		}
		if item.Status != ItemOK {
			return ErrItemAlreadyChanged
		}
		actor := pgtype.UUID{Bytes: in.ActorUserID, Valid: true}

		if in.Substitute == nil {
			if err := q.SetOrderItemState(ctx, db.SetOrderItemStateParams{ID: item.ID, Status: ItemRemoved, ChargedTotal: 0}); err != nil {
				return err
			}
			note := fmt.Sprintf("%s × %d unavailable - removed", item.ProductName, item.Quantity)
			if err := addEvent(ctx, q, order, actor, "removed", note); err != nil {
				return err
			}
			if err := queueNotification(ctx, q, order.ID, "item_removed", map[string]any{"item_id": item.ID, "reason": "unavailable"}); err != nil {
				return err
			}
			return s.afterItemChange(ctx, q, order)
		}

		if !order.AllowSubstitutions {
			return ErrSubstitutionsNotAllowed
		}
		if in.Substitute.ProductID == item.ProductID {
			return ErrSubstituteSameProduct
		}
		rows, err := q.ListBranchProductsByID(ctx, db.ListBranchProductsByIDParams{BranchID: in.BranchID, Ids: []uuid.UUID{in.Substitute.ProductID}})
		if err != nil {
			return err
		}
		if len(rows) == 0 || rows[0].Price == nil {
			return ErrSubstituteUnavailable
		}
		product := rows[0]
		name := "Product"
		if product.Name != nil {
			name = *product.Name
		}
		price := *product.Price
		lineTotal := min(item.LineTotal, price*int64(in.Substitute.Quantity))

		sub, err := q.InsertSubstitution(ctx, db.InsertSubstitutionParams{
			OrderID:         order.ID,
			OrderItemID:     item.ID,
			ProductID:       product.ID,
			ProductName:     name,
			ProductPlu:      product.Plu,
			UnitPrice:       price,
			Quantity:        int32(in.Substitute.Quantity),
			LineTotal:       lineTotal,
			ExpiresAt:       pgtype.Timestamptz{Time: s.now().Add(s.timeouts.Substitution), Valid: true},
			CreatedByUserID: actor,
		})
		if err != nil {
			return err
		}
		// The line keeps costing what was ordered until the customer answers.
		if err := q.SetOrderItemState(ctx, db.SetOrderItemStateParams{ID: item.ID, Status: ItemAwaitingCustomer, ChargedTotal: item.ChargedTotal}); err != nil {
			return err
		}
		note := fmt.Sprintf("Offered %s × %d instead of %s × %d", name, in.Substitute.Quantity, item.ProductName, item.Quantity)
		if err := addEvent(ctx, q, order, actor, "substitution", note); err != nil {
			return err
		}
		if err := queueNotification(ctx, q, order.ID, "substitution_proposed", map[string]any{"substitution_id": sub.ID}); err != nil {
			return err
		}
		return s.afterItemChange(ctx, q, order)
	})
}

// WithdrawSubstitution takes back an offer the customer hasn't answered;
// the line is back to normal.
func (s *Service) WithdrawSubstitution(ctx context.Context, branchID, orderID, itemID, actorUserID uuid.UUID) (Order, error) {
	return s.inOrderTx(ctx, branchID, orderID, func(q *db.Queries, order db.Order) error {
		item, err := q.GetOrderItemForOrder(ctx, db.GetOrderItemForOrderParams{ID: itemID, OrderID: order.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrItemNotFound
		}
		if err != nil {
			return err
		}
		sub, err := q.GetLiveSubstitutionForItem(ctx, item.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoLiveSubstitution
		}
		if err != nil {
			return err
		}
		if n, err := q.SetSubstitutionStatus(ctx, db.SetSubstitutionStatusParams{ID: sub.ID, Status: "withdrawn"}); err != nil {
			return err
		} else if n == 0 {
			return ErrSubstitutionClosed
		}
		if err := q.SetOrderItemState(ctx, db.SetOrderItemStateParams{ID: item.ID, Status: ItemOK, ChargedTotal: item.LineTotal}); err != nil {
			return err
		}
		note := fmt.Sprintf("Withdrew the offer of %s for %s", sub.ProductName, item.ProductName)
		if err := addEvent(ctx, q, order, pgtype.UUID{Bytes: actorUserID, Valid: true}, "substitution", note); err != nil {
			return err
		}
		return s.afterItemChange(ctx, q, order)
	})
}

// RespondToSubstitution records the customer's answer. The caller has
// already checked the customer may see this order.
func (s *Service) RespondToSubstitution(ctx context.Context, branchID, orderID, substitutionID uuid.UUID, accept bool) (Order, error) {
	return s.inOrderTx(ctx, branchID, orderID, func(q *db.Queries, order db.Order) error {
		sub, err := q.GetSubstitutionForOrder(ctx, db.GetSubstitutionForOrderParams{ID: substitutionID, OrderID: order.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSubstitutionNotFound
		}
		if err != nil {
			return err
		}
		if sub.Status != "proposed" || !sub.ExpiresAt.Time.After(s.now()) ||
			(order.Status != "accepted" && order.Status != "picking") {
			return ErrSubstitutionClosed
		}
		status, itemStatus, charged, verb := "declined", ItemRemoved, int64(0), "declined"
		if accept {
			status, itemStatus, charged, verb = "accepted", ItemSubstituted, sub.LineTotal, "accepted"
		}
		if n, err := q.SetSubstitutionStatus(ctx, db.SetSubstitutionStatusParams{ID: sub.ID, Status: status}); err != nil {
			return err
		} else if n == 0 {
			return ErrSubstitutionClosed
		}
		if err := q.SetOrderItemState(ctx, db.SetOrderItemStateParams{ID: sub.OrderItemID, Status: itemStatus, ChargedTotal: charged}); err != nil {
			return err
		}
		note := fmt.Sprintf("Customer %s %s × %d", verb, sub.ProductName, sub.Quantity)
		if err := addEvent(ctx, q, order, pgtype.UUID{}, "substitution", note); err != nil {
			return err
		}
		return s.afterItemChange(ctx, q, order)
	})
}

// ExpireSubstitutions removes lines whose substitute wasn't answered in
// time. Each one is its own transaction, so one failure doesn't block the
// rest.
func (s *Service) ExpireSubstitutions(ctx context.Context) (int, error) {
	due, err := s.queries.ListDueSubstitutions(ctx)
	if err != nil {
		return 0, err
	}
	expired := 0
	for _, d := range due {
		_, err := s.inOrderTx(ctx, uuid.Nil, d.OrderID, func(q *db.Queries, order db.Order) error {
			sub, err := q.GetSubstitutionForOrder(ctx, db.GetSubstitutionForOrderParams{ID: d.ID, OrderID: order.ID})
			if err != nil {
				return err
			}
			if n, err := q.SetSubstitutionStatus(ctx, db.SetSubstitutionStatusParams{ID: sub.ID, Status: "expired"}); err != nil || n == 0 {
				return err // answered in the meantime: nothing to do
			}
			item, err := q.GetOrderItemForOrder(ctx, db.GetOrderItemForOrderParams{ID: sub.OrderItemID, OrderID: order.ID})
			if err != nil {
				return err
			}
			if err := q.SetOrderItemState(ctx, db.SetOrderItemStateParams{ID: item.ID, Status: ItemRemoved, ChargedTotal: 0}); err != nil {
				return err
			}
			note := fmt.Sprintf("No answer about %s in time - %s removed", sub.ProductName, item.ProductName)
			if err := addEvent(ctx, q, order, pgtype.UUID{}, "removed", note); err != nil {
				return err
			}
			if err := queueNotification(ctx, q, order.ID, "item_removed", map[string]any{"item_id": item.ID, "reason": "no_answer"}); err != nil {
				return err
			}
			expired++
			return s.afterItemChange(ctx, q, order)
		})
		if err != nil {
			s.logger.Error("expire substitution failed", "substitution_id", d.ID, "error", err)
		}
	}
	return expired, nil
}

// afterItemChange re-totals the order, or cancels it if nothing is left.
func (s *Service) afterItemChange(ctx context.Context, q *db.Queries, order db.Order) error {
	counts, err := q.CountOrderItemStates(ctx, order.ID)
	if err != nil {
		return err
	}
	if _, err := q.RecalculateOrderTotals(ctx, order.ID); err != nil {
		return err
	}
	if counts.Live > 0 {
		return nil
	}
	reason := allItemsGoneReason
	if _, err := q.TransitionOrderStatus(ctx, db.TransitionOrderStatusParams{
		ID: order.ID, BranchID: order.BranchID, ExpectedStatus: order.Status,
		NewStatus: "cancelled", RejectionReason: &reason,
	}); err != nil {
		return fmt.Errorf("cancel empty order: %w", err)
	}
	from := order.Status
	return q.InsertOrderStatusEvent(ctx, db.InsertOrderStatusEventParams{
		OrderID: order.ID, FromStatus: &from, ToStatus: "cancelled", Note: &reason, Kind: "status",
	})
}

// inOrderTx runs fn with the order locked, then returns the order as it is
// afterwards. branchID uuid.Nil skips the branch check (background jobs).
func (s *Service) inOrderTx(ctx context.Context, branchID, orderID uuid.UUID, fn func(q *db.Queries, order db.Order) error) (Order, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Order{}, err
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)

	order, err := q.LockOrder(ctx, orderID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && branchID != uuid.Nil && order.BranchID != branchID) {
		return Order{}, ErrOrderNotFound
	}
	if err != nil {
		return Order{}, err
	}
	if err := fn(q, order); err != nil {
		return Order{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Order{}, err
	}
	row, err := s.queries.GetOrderByID(ctx, orderID)
	if err != nil {
		return Order{}, err
	}
	return s.withItems(ctx, row)
}

// addEvent records a non-status history entry (from = to = current status).
func addEvent(ctx context.Context, q *db.Queries, order db.Order, actor pgtype.UUID, kind, note string) error {
	status := order.Status
	return q.InsertOrderStatusEvent(ctx, db.InsertOrderStatusEventParams{
		OrderID: order.ID, FromStatus: &status, ToStatus: status, ActorUserID: actor, Note: &note, Kind: kind,
	})
}

// queueNotification adds a row to the outbox in the caller's transaction,
// and wakes the sender once it commits.
func queueNotification(ctx context.Context, q *db.Queries, orderID uuid.UUID, kind string, detail map[string]any) error {
	raw, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	if err := q.InsertNotification(ctx, db.InsertNotificationParams{OrderID: orderID, Kind: kind, Detail: raw}); err != nil {
		return err
	}
	return q.NotifyOutbox(ctx)
}
