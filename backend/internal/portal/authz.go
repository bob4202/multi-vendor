// Package portal is the vendor portal's business logic: logging vendor users
// in, resolving sessions, deciding what each role may do, and moving orders
// through their status lifecycle. HTTP handlers stay thin and call into it.
package portal

import (
	"github.com/google/uuid"

	"ecommerce/backend/internal/db"
)

type Role string

const (
	RoleOwner   Role = "owner"
	RoleManager Role = "manager"
	RoleStaff   Role = "staff"
)

func (r Role) Valid() bool {
	return r == RoleOwner || r == RoleManager || r == RoleStaff
}

// Action is something a vendor user can try to do inside a branch they have
// access to. Which branches they can reach is decided separately (owners:
// all of their vendor's; others: assigned ones).
type Action int

const (
	ViewOrders Action = iota
	UpdateOrderStatus
	ViewStoreInfo
	EditStoreInfo
	ViewProducts
	TriggerSync
	RefundOrders
)

// permissions is the single source of truth for role capabilities. Owners
// are not listed: they can do everything.
var permissions = map[Role]map[Action]bool{
	RoleManager: {
		ViewOrders: true, UpdateOrderStatus: true,
		ViewStoreInfo: true, EditStoreInfo: true,
		ViewProducts: true, TriggerSync: true,
		RefundOrders: true,
	},
	RoleStaff: {
		ViewOrders: true, UpdateOrderStatus: true,
		ViewStoreInfo: true,
		ViewProducts:  true,
	},
}

func (r Role) Can(a Action) bool {
	if r == RoleOwner {
		return true
	}
	return permissions[r][a]
}

// Principal is the authenticated vendor user behind a request.
type Principal struct {
	SessionID uuid.UUID
	User      db.VendorUser
	Vendor    db.Vendor
}

func (p Principal) Role() Role {
	return Role(p.User.Role)
}

func (p Principal) Can(a Action) bool {
	return p.Role().Can(a)
}
