package portal

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"ecommerce/backend/internal/auth"
	"ecommerce/backend/internal/crypto"
	"ecommerce/backend/internal/db"
)

// Account management, used by the platform admin API (and later by owners
// managing their own staff).

var (
	ErrSlugTaken  = errors.New("a vendor with that slug already exists")
	ErrEmailTaken = errors.New("a user with that email already exists")
)

var slugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func (s *Service) CreateVendor(ctx context.Context, slug, name string) (db.Vendor, error) {
	slug, name = strings.TrimSpace(slug), strings.TrimSpace(name)
	var issues []Issue
	if !slugRe.MatchString(slug) {
		issues = append(issues, Issue{"slug", "Slug must be lowercase letters, numbers and single hyphens"})
	}
	if name == "" {
		issues = append(issues, Issue{"name", "Name is required"})
	}
	if len(issues) > 0 {
		return db.Vendor{}, &ValidationError{Issues: issues}
	}

	v, err := s.queries.CreateVendor(ctx, db.CreateVendorParams{Slug: slug, Name: name})
	if isUniqueViolation(err) {
		return db.Vendor{}, ErrSlugTaken
	}
	return v, err
}

type CreateUserInput struct {
	VendorID  uuid.UUID
	Email     string
	Name      string
	Role      Role
	Password  string      // empty = invite: the user sets their own via the link
	BranchIDs []uuid.UUID // required for managers and staff; ignored for owners
	InvitedBy *uuid.UUID  // platform admin issuing the invite, if any
}

// CreateUser adds a vendor user. Without a password it also issues an
// invite (returned non-nil) whose link lets them choose one.
func (s *Service) CreateUser(ctx context.Context, in CreateUserInput) (db.VendorUser, *Invite, error) {
	in.Email = NormalizeEmail(in.Email)
	in.Name = strings.TrimSpace(in.Name)

	var issues []Issue
	if !strings.Contains(in.Email, "@") {
		issues = append(issues, Issue{"email", "Enter a valid email address"})
	}
	if in.Name == "" {
		issues = append(issues, Issue{"name", "Name is required"})
	}
	if !in.Role.Valid() {
		issues = append(issues, Issue{"role", "Role must be owner, manager or staff"})
	}
	if in.Password != "" {
		if err := auth.ValidatePassword(in.Password); err != nil {
			issues = append(issues, Issue{"password", err.Error()})
		}
	}
	if in.Role != RoleOwner && len(in.BranchIDs) == 0 {
		issues = append(issues, Issue{"branch_ids", "Managers and staff need at least one branch"})
	}
	if len(issues) > 0 {
		return db.VendorUser{}, nil, &ValidationError{Issues: issues}
	}

	var hash *string
	if in.Password != "" {
		h, err := crypto.HashPassword(in.Password)
		if err != nil {
			return db.VendorUser{}, nil, err
		}
		hash = &h
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.VendorUser{}, nil, err
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)

	user, err := q.CreateVendorUser(ctx, db.CreateVendorUserParams{
		VendorID: in.VendorID, Email: in.Email, Name: in.Name, PasswordHash: hash, Role: string(in.Role),
	})
	if isUniqueViolation(err) {
		return db.VendorUser{}, nil, ErrEmailTaken
	}
	if err != nil {
		return db.VendorUser{}, nil, err
	}

	if in.Role != RoleOwner {
		for _, branchID := range in.BranchIDs {
			err := q.AssignVendorUserBranch(ctx, db.AssignVendorUserBranchParams{VendorID: in.VendorID, UserID: user.ID, BranchID: branchID})
			if isForeignKeyViolation(err) {
				return db.VendorUser{}, nil, &ValidationError{Issues: []Issue{{"branch_ids", fmt.Sprintf("Branch %s doesn't belong to this vendor", branchID)}}}
			}
			if err != nil {
				return db.VendorUser{}, nil, err
			}
		}
	}

	var invite *Invite
	if hash == nil {
		inv, err := s.issueInvite(ctx, q, user.ID, in.InvitedBy)
		if err != nil {
			return db.VendorUser{}, nil, err
		}
		invite = &inv
	}

	if err := tx.Commit(ctx); err != nil {
		return db.VendorUser{}, nil, err
	}
	return user, invite, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}
