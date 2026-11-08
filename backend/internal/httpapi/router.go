// Package httpapi exposes the HTTP API:
//   - /v1/admin: platform admin (admin session or X-Admin-Key) - vendors, branches, credentials, syncs
//   - /v1/vendor: vendor portal (session token) - a vendor's own branches and orders
//   - /v1/store: storefront (storefront key) - one branch's catalogue
package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"ecommerce/backend/internal/auth"
	"ecommerce/backend/internal/crypto"
	"ecommerce/backend/internal/customer"
	"ecommerce/backend/internal/db"
	"ecommerce/backend/internal/events"
	"ecommerce/backend/internal/payment"
	"ecommerce/backend/internal/platform"
	"ecommerce/backend/internal/portal"
	"ecommerce/backend/internal/push"
	"ecommerce/backend/internal/store"
	syncpkg "ecommerce/backend/internal/sync"
)

// Deps is everything the HTTP layer needs from the rest of the service.
type Deps struct {
	Queries   *db.Queries
	Engine    *syncpkg.Engine
	SecretBox *crypto.SecretBox
	Orders    *store.Service
	Portal    *portal.Service
	Platform  *platform.Service
	Customers *customer.Service
	Events    *events.Hub
	Push      *push.Sender // nil = push notifications not configured
	AdminKey  string       // shared key for scripts; admins in the UI use sessions
	Payments  *payment.Service
	Logger    *slog.Logger
}

type Server struct {
	queries   *db.Queries
	engine    *syncpkg.Engine
	secretBox *crypto.SecretBox
	orders    *store.Service
	portal    *portal.Service
	platform  *platform.Service
	customers *customer.Service
	events    *events.Hub
	push      *push.Sender
	logger    *slog.Logger
	payments  *payment.Service
	adminKey  string
}

func NewRouter(d Deps) http.Handler {
	s := &Server{
		queries: d.Queries, engine: d.Engine, secretBox: d.SecretBox, orders: d.Orders,
		portal: d.Portal, platform: d.Platform, logger: d.Logger, adminKey: d.AdminKey,
		customers: d.Customers, events: d.Events, push: d.Push, payments: d.Payments,
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", s.handleHealth)

	// The fake payment provider's hosted page, for local development only.
	if s.payments != nil && s.payments.HasProvider("fake") {
		r.Get("/fake-pay/{ref}", s.handleFakePayPage)
		r.Post("/fake-pay/{ref}", s.handleFakePaySubmit)
	}

	// Versioned surface - /healthz stays outside it since load balancers and
	// orchestrators expect a stable, unversioned health check.
	r.Route("/v1", func(r chi.Router) {
		// Called by vendors' payment providers; signed, no storefront key.
		r.Post("/webhooks/{provider}/{accountID}", s.handlePaymentWebhook)

		r.Route("/admin", func(r chi.Router) {
			r.Post("/auth/login", s.handleAdminLogin)

			r.Group(func(r chi.Router) {
				r.Use(s.requireAdmin)

				r.Post("/auth/logout", s.handleAdminLogout)
				r.Get("/me", s.handleAdminMe)
				r.Get("/overview", s.handleAdminOverview)

				r.Get("/inventory-providers", s.handleListInventoryProviders)
				r.Route("/branches", func(r chi.Router) {
					r.Get("/", s.handleListBranches)
					r.Post("/", s.handleCreateBranch)

					r.Route("/{branchID}", func(r chi.Router) {
						r.Get("/", s.handleGetBranch)
						r.Put("/", s.handleUpdateBranch)
						r.Post("/sync", s.handleTriggerSync)
						r.Get("/sync-state", s.handleSyncState)

						r.Post("/storefront-key", s.handleRotateStorefrontKey)
						r.Delete("/storefront-key", s.handleRevokeStorefrontKey)

						r.Get("/products", s.handleAdminListBranchProducts)
						r.Get("/products/{productID}", s.handleAdminGetBranchProduct)
						r.Get("/categories", s.handleAdminListBranchCategories)

						r.Get("/info", s.handleAdminGetStoreInfo)
						r.Put("/info", s.handleAdminSaveStoreInfo)

						r.Get("/orders", s.handleAdminListActiveOrders)
						r.Post("/orders/{orderID}/accept", s.handleAdminAcceptOrder)
						r.Post("/orders/{orderID}/reject", s.handleAdminRejectOrder)
					})
				})

				r.Route("/categories", func(r chi.Router) {
					r.Get("/", s.handleListCategories)
					r.Post("/", s.handleUpsertCategory)
				})

				r.Route("/vendors", func(r chi.Router) {
					r.Get("/", s.handleListVendors)
					r.Post("/", s.handleCreateVendor)
					r.Get("/{vendorID}", s.handleGetVendor)
					r.Patch("/{vendorID}", s.handleUpdateVendor)
					r.Post("/{vendorID}/users", s.handleCreateVendorUser)
					r.Post("/{vendorID}/users/{userID}/invite", s.handleReissueInvite)
				})
			})
		})

		r.Route("/vendor", func(r chi.Router) {
			r.Post("/auth/login", s.handleVendorLogin)
			r.Get("/invites/{token}", s.handleGetInvite)
			r.Post("/invites/{token}/accept", s.handleAcceptInvite)

			r.Group(func(r chi.Router) {
				r.Use(s.requireVendorSession)

				r.Post("/auth/logout", s.handleVendorLogout)
				r.Get("/me", s.handleVendorMe)

				r.Route("/branches/{branchID}", func(r chi.Router) {
					r.Use(s.requireBranchAccess)

					r.With(requirePermission(portal.ViewOrders)).Get("/orders", s.handleVendorListOrders)
					r.With(requirePermission(portal.ViewOrders)).Get("/orders/lookup", s.handleVendorLookupOrder)
					r.With(requirePermission(portal.ViewOrders)).Get("/events", s.handleVendorBranchEvents)
					r.With(requirePermission(portal.ViewOrders)).Get("/orders/{orderID}", s.handleVendorGetOrder)
					r.With(requirePermission(portal.UpdateOrderStatus)).Post("/orders/{orderID}/status", s.handleVendorChangeOrderStatus)
					r.With(requirePermission(portal.UpdateOrderStatus)).Post("/orders/{orderID}/items/{itemID}/unavailable", s.handleVendorItemUnavailable)
					r.With(requirePermission(portal.UpdateOrderStatus)).Delete("/orders/{orderID}/items/{itemID}/substitution", s.handleVendorWithdrawSubstitution)
					r.With(requirePermission(portal.RefundOrders)).Post("/orders/{orderID}/refunds", s.handleVendorRefund)

					r.With(requirePermission(portal.ViewStoreInfo)).Get("/info", s.handleVendorGetStoreInfo)
					r.With(requirePermission(portal.EditStoreInfo)).Put("/info", s.handleVendorSaveStoreInfo)
					r.With(requirePermission(portal.ViewStoreInfo)).Get("/closures", s.handleVendorListClosures)
					r.With(requirePermission(portal.EditStoreInfo)).Post("/closures", s.handleVendorAddClosure)
					r.With(requirePermission(portal.EditStoreInfo)).Delete("/closures/{closureID}", s.handleVendorDeleteClosure)

					r.With(requirePermission(portal.ViewProducts)).Get("/products", s.handleVendorListProducts)
					r.With(requirePermission(portal.ViewProducts)).Get("/categories", s.handleVendorListCategories)
					r.With(requirePermission(portal.ViewProducts)).Get("/sync-state", s.handleVendorSyncState)
					r.With(requirePermission(portal.TriggerSync)).Post("/sync", s.handleVendorTriggerSync)
				})
			})
		})

		r.Route("/store", func(r chi.Router) {
			r.Use(s.requireStorefrontKey)
			r.Use(s.withCustomer)

			r.Get("/products", s.handleStorefrontListProducts)
			r.Get("/products/{productID}", s.handleStorefrontGetProduct)
			r.Get("/categories", s.handleStorefrontListCategories)
			r.Get("/branch", s.handleStorefrontGetBranch)

			r.Post("/orders", s.handleStorefrontPlaceOrder)
			r.Get("/orders/{orderID}", s.handleStorefrontGetOrder)
			r.Post("/orders/{orderID}/substitutions/{substitutionID}", s.handleStorefrontRespondSubstitution)
			r.Get("/orders/{orderID}/events", s.handleStorefrontOrderEvents)
			r.Post("/orders/{orderID}/payment-session", s.handleStorefrontStartPaymentSession)

			r.Get("/push/config", s.handlePushConfig)
			r.Post("/push-subscriptions", s.handleSavePushSubscription)
			r.Delete("/push-subscriptions", s.handleDeletePushSubscription)

			r.Post("/auth/code", s.handleCustomerSendCode)
			r.Post("/auth/verify", s.handleCustomerVerify)
			r.Post("/auth/logout", s.handleCustomerLogout)
			r.Group(func(r chi.Router) {
				r.Use(requireCustomer)
				r.Get("/me", s.handleCustomerMe)
				r.Patch("/me", s.handleCustomerUpdateMe)
				r.Get("/me/orders", s.handleCustomerOrders)
			})
		})
	})

	return r
}

type adminActorContextKey struct{}

// requireAdmin accepts either the shared X-Admin-Key (scripts, curl) or a
// platform admin's session token (the admin UI). The actor goes into the
// context so actions can be audited against the right person.
func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var actor platform.Actor
		if key := r.Header.Get("X-Admin-Key"); key != "" {
			if subtle.ConstantTimeCompare([]byte(key), []byte(s.adminKey)) != 1 {
				writeError(w, http.StatusUnauthorized, "invalid X-Admin-Key header")
				return
			}
		} else {
			admin, err := s.platform.Authenticate(r.Context(), bearerToken(r))
			if errors.Is(err, auth.ErrUnauthenticated) {
				writeError(w, http.StatusUnauthorized, "invalid or expired admin session")
				return
			}
			if err != nil {
				s.internalError(w, r, err)
				return
			}
			actor.AdminID = &admin.ID
			r = r.WithContext(context.WithValue(r.Context(), adminContextKey{}, admin))
		}
		ctx := context.WithValue(r.Context(), adminActorContextKey{}, actor)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func adminActor(r *http.Request) platform.Actor {
	a, _ := r.Context().Value(adminActorContextKey{}).(platform.Actor)
	return a
}

// audit records an admin action. A failure to record is logged, never
// returned - the action itself already happened.
func (s *Server) audit(r *http.Request, action, targetType, targetID, summary string) {
	if err := s.platform.Audit(r.Context(), adminActor(r), action, targetType, targetID, summary); err != nil {
		s.logger.Error("audit failed", "action", action, "target_id", targetID, "error", err)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func branchIDFromRequest(r *http.Request) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, "branchID"))
}

func productIDFromRequest(r *http.Request) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, "productID"))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// writeIssues is the 422 shape shared with orders: a summary plus
// field-level problems the client can show next to each input.
func writeIssues(w http.ResponseWriter, msg string, issues []portal.Issue) {
	writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": msg, "issues": issues})
}

// internalError logs the real error and returns a generic message, so
// database or driver details never leak to clients.
func (s *Server) internalError(w http.ResponseWriter, r *http.Request, err error) {
	s.logger.Error("request failed", "method", r.Method, "path", r.URL.Path, "request_id", middleware.GetReqID(r.Context()), "error", err)
	writeError(w, http.StatusInternalServerError, "something went wrong")
}

const maxJSONBody = 64 << 10

// decodeJSON reads a size-capped JSON body and rejects unknown fields, so a
// typo in a field name fails loudly instead of being silently ignored.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return errors.New("request body too large")
		}
		return fmt.Errorf("invalid json body: %w", err)
	}
	return nil
}
