package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"ecommerce/backend/internal/db"
	"ecommerce/backend/internal/platform"
	"ecommerce/backend/internal/portal"
)

// TestVendorPortalTenancy checks the security boundary end to end through
// the real router: a vendor user only ever reaches their own branches, and
// logging out kills the token. Requires DATABASE_URL (a migrated database).
func TestVendorPortalTenancy(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	q := db.New(pool)
	svc := portal.NewService(pool)
	suffix := strings.ToLower(uuid.NewString()[:8])
	const password = "integration-test-pw"

	vendorA, err := svc.CreateVendor(ctx, "test-a-"+suffix, "Vendor A")
	if err != nil {
		t.Fatalf("create vendor A: %v", err)
	}
	vendorB, err := svc.CreateVendor(ctx, "test-b-"+suffix, "Vendor B")
	if err != nil {
		t.Fatalf("create vendor B: %v", err)
	}
	newBranch := func(vendorID uuid.UUID, name string) db.Branch {
		b, err := q.CreateBranch(ctx, db.CreateBranchParams{
			Name: name, Host: name + "-" + suffix, ApiUsername: "u", ApiKeyCipher: []byte("x"),
			IsActive: true, AllowedSecondaryDepartmentUuids: []string{}, AllowedProductRefs: []string{}, VendorID: vendorID,
		})
		if err != nil {
			t.Fatalf("create branch %s: %v", name, err)
		}
		return b
	}
	a1 := newBranch(vendorA.ID, "a1")
	a2 := newBranch(vendorA.ID, "a2")
	b1 := newBranch(vendorB.ID, "b1")

	staffEmail := "staff-" + suffix + "@a.example"
	if _, _, err := svc.CreateUser(ctx, portal.CreateUserInput{
		VendorID: vendorA.ID, Email: staffEmail, Name: "Staff A", Role: portal.RoleStaff,
		Password: password, BranchIDs: []uuid.UUID{a1.ID},
	}); err != nil {
		t.Fatalf("create staff: %v", err)
	}
	// The composite FK must refuse another vendor's branch outright.
	if _, _, err := svc.CreateUser(ctx, portal.CreateUserInput{
		VendorID: vendorA.ID, Email: "sneaky-" + suffix + "@a.example", Name: "Sneaky", Role: portal.RoleStaff,
		Password: password, BranchIDs: []uuid.UUID{b1.ID},
	}); err == nil {
		t.Fatal("expected assigning vendor B's branch to a vendor A user to fail")
	}

	srv := httptest.NewServer(NewRouter(Deps{
		Queries: q, Portal: svc, Platform: platform.NewService(pool),
		AdminKey: "admin-key", Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}))
	defer srv.Close()

	call := func(method, path, token, body string) (int, map[string]any) {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer res.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(res.Body).Decode(&out)
		return res.StatusCode, out
	}

	status, body := call("POST", "/v1/vendor/auth/login", "", `{"email":"`+strings.ToUpper(staffEmail)+`","password":"`+password+`"}`)
	if status != http.StatusOK {
		t.Fatalf("login: got %d %v", status, body)
	}
	token, _ := body["token"].(string)

	if status, body := call("GET", "/v1/vendor/me", token, ""); status != http.StatusOK {
		t.Fatalf("me: got %d", status)
	} else if branches := body["branches"].([]any); len(branches) != 1 || branches[0].(map[string]any)["id"] != a1.ID.String() {
		t.Errorf("me should list only branch a1, got %v", branches)
	}

	cases := []struct {
		name   string
		branch uuid.UUID
		want   int
	}{
		{"assigned branch", a1.ID, http.StatusOK},
		{"own vendor, unassigned branch", a2.ID, http.StatusNotFound},
		{"other vendor's branch", b1.ID, http.StatusNotFound},
	}
	for _, c := range cases {
		if status, _ := call("GET", "/v1/vendor/branches/"+c.branch.String()+"/orders", token, ""); status != c.want {
			t.Errorf("%s: got %d, want %d", c.name, status, c.want)
		}
	}

	if status, _ := call("PUT", "/v1/vendor/branches/"+a1.ID.String()+"/info", token, `{}`); status != http.StatusForbidden {
		t.Errorf("staff editing store info: got %d, want 403", status)
	}

	if status, _ := call("POST", "/v1/vendor/auth/logout", token, ""); status != http.StatusNoContent {
		t.Errorf("logout: got %d", status)
	}
	if status, _ := call("GET", "/v1/vendor/me", token, ""); status != http.StatusUnauthorized {
		t.Errorf("token after logout: got %d, want 401", status)
	}
}
