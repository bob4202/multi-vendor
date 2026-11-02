package httpapi

import (
	"context"
	"encoding/base64"
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

	"ecommerce/backend/internal/crypto"
	"ecommerce/backend/internal/db"
	"ecommerce/backend/internal/inventory"
	inventoryfake "ecommerce/backend/internal/inventory/fake"
	"ecommerce/backend/internal/platform"
	"ecommerce/backend/internal/portal"
	syncpkg "ecommerce/backend/internal/sync"
)

// TestAdminOnboardingFlow drives the admin API the way the admin UI does:
// admin login, create a vendor and branch, invite an owner, the owner
// accepts and can log in, and every step shows up in the overview and the
// audit trail. Requires DATABASE_URL (a migrated database).
func TestAdminOnboardingFlow(t *testing.T) {
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
	platformSvc := platform.NewService(pool)
	suffix := strings.ToLower(uuid.NewString()[:8])
	const adminKey = "test-admin-key"
	const password = "integration-test-pw"

	adminEmail := "admin-" + suffix + "@test.example"
	if _, err := platformSvc.CreateAdmin(ctx, adminEmail, "Test Admin", password); err != nil {
		t.Fatalf("create admin: %v", err)
	}

	box, err := crypto.NewSecretBox(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	engine := syncpkg.NewEngine(pool, box, inventory.Registry{inventoryfake.Name: inventoryfake.New}, logger)
	srv := httptest.NewServer(NewRouter(Deps{
		Queries: q, SecretBox: box, Engine: engine, Portal: portal.NewService(pool), Platform: platformSvc,
		AdminKey: adminKey, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}))
	defer srv.Close()

	type headers map[string]string
	call := func(method, path string, h headers, body string) (int, map[string]any) {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		for k, v := range h {
			req.Header.Set(k, v)
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
	bearer := func(token string) headers { return headers{"Authorization": "Bearer " + token} }

	// --- admin auth ---------------------------------------------------------
	status, body := call("POST", "/v1/admin/auth/login", nil, `{"email":"`+adminEmail+`","password":"`+password+`"}`)
	if status != http.StatusOK {
		t.Fatalf("admin login: %d %v", status, body)
	}
	admin := bearer(body["token"].(string))

	if status, _ := call("GET", "/v1/admin/vendors", nil, ""); status != http.StatusUnauthorized {
		t.Errorf("no credentials: got %d, want 401", status)
	}
	if status, _ := call("GET", "/v1/admin/vendors", headers{"X-Admin-Key": adminKey}, ""); status != http.StatusOK {
		t.Errorf("admin key: got %d, want 200", status)
	}
	if status, _ := call("GET", "/v1/admin/me", admin, ""); status != http.StatusOK {
		t.Errorf("admin session: got %d, want 200", status)
	}

	// --- onboarding ------------------------------------------------------------
	status, body = call("POST", "/v1/admin/vendors", admin, `{"slug":"onboard-`+suffix+`","name":"Onboard `+suffix+`"}`)
	if status != http.StatusCreated {
		t.Fatalf("create vendor: %d %v", status, body)
	}
	vendorID := body["id"].(string)

	status, body = call("POST", "/v1/admin/branches", admin,
		`{"name":"Branch `+suffix+`","host":"onboard-`+suffix+`","api_username":"u","api_key":"k","vendor_id":"`+vendorID+`","allowed_secondary_department_uuids":["dept-x"]}`)
	if status != http.StatusCreated {
		t.Fatalf("create branch: %d %v", status, body)
	}
	branchID := body["id"].(string)

	ownerEmail := "owner-" + suffix + "@test.example"
	status, body = call("POST", "/v1/admin/vendors/"+vendorID+"/users", admin,
		`{"email":"`+ownerEmail+`","name":"Owner","role":"owner"}`)
	if status != http.StatusCreated {
		t.Fatalf("invite owner: %d %v", status, body)
	}
	firstToken := body["invite"].(map[string]any)["token"].(string)

	// The vendor detail shows the user as invited and can't log in yet.
	_, body = call("GET", "/v1/admin/vendors/"+vendorID, admin, "")
	users := body["users"].([]any)
	if len(users) != 1 || users[0].(map[string]any)["status"] != "invited" {
		t.Errorf("expected one invited user, got %v", users)
	}
	if status, _ := call("POST", "/v1/vendor/auth/login", nil, `{"email":"`+ownerEmail+`","password":"anything-long-enough"}`); status != http.StatusUnauthorized {
		t.Errorf("login before accepting invite: got %d, want 401", status)
	}

	// Resending replaces the link: the first one stops working.
	userID := users[0].(map[string]any)["id"].(string)
	status, body = call("POST", "/v1/admin/vendors/"+vendorID+"/users/"+userID+"/invite", admin, "")
	if status != http.StatusCreated {
		t.Fatalf("resend invite: %d %v", status, body)
	}
	token := body["token"].(string)
	if status, _ := call("GET", "/v1/vendor/invites/"+firstToken, nil, ""); status != http.StatusGone {
		t.Errorf("superseded invite: got %d, want 410", status)
	}
	if status, _ := call("GET", "/v1/vendor/invites/vi_doesnotexist", nil, ""); status != http.StatusNotFound {
		t.Errorf("unknown invite: got %d, want 404", status)
	}

	status, body = call("GET", "/v1/vendor/invites/"+token, nil, "")
	if status != http.StatusOK || body["email"] != ownerEmail {
		t.Fatalf("get invite: %d %v", status, body)
	}
	if status, _ := call("POST", "/v1/vendor/invites/"+token+"/accept", nil, `{"password":"short"}`); status != http.StatusUnprocessableEntity {
		t.Errorf("short password: got %d, want 422", status)
	}
	status, body = call("POST", "/v1/vendor/invites/"+token+"/accept", nil, `{"password":"`+password+`"}`)
	if status != http.StatusOK || body["token"] == nil {
		t.Fatalf("accept invite: %d %v", status, body)
	}
	if status, _ := call("POST", "/v1/vendor/invites/"+token+"/accept", nil, `{"password":"`+password+`"}`); status != http.StatusGone {
		t.Errorf("reusing an invite: got %d, want 410", status)
	}
	status, body = call("POST", "/v1/vendor/auth/login", nil, `{"email":"`+ownerEmail+`","password":"`+password+`"}`)
	if status != http.StatusOK {
		t.Fatalf("owner login after accepting: %d %v", status, body)
	}
	vendorSession := body["token"].(string)

	// A vendor session must never open the admin API.
	if status, _ := call("GET", "/v1/admin/vendors", bearer(vendorSession), ""); status != http.StatusUnauthorized {
		t.Errorf("vendor session on admin API: got %d, want 401", status)
	}

	// --- overview + audit --------------------------------------------------------
	_, body = call("GET", "/v1/admin/overview", admin, "")
	kinds := map[string]bool{}
	for _, it := range body["attention"].([]any) {
		item := it.(map[string]any)
		if item["branch_id"] == branchID {
			kinds[item["kind"].(string)] = true
		}
	}
	for _, k := range []string{platform.KindNoStoreDetails, platform.KindNoStorefrontKey, platform.KindNeverSynced} {
		if !kinds[k] {
			t.Errorf("overview should flag new branch with %s, got %v", k, kinds)
		}
	}

	var actors int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM admin_audit_events e JOIN platform_admins a ON a.id = e.admin_id
		WHERE a.email = $1 AND e.action IN ('vendor.created', 'branch.created', 'user.created', 'user.invite_resent')`,
		adminEmail,
	).Scan(&actors); err != nil {
		t.Fatal(err)
	}
	if actors != 4 {
		t.Errorf("expected 4 audited actions by the admin, got %d", actors)
	}

	// Deactivating the vendor logs its users out immediately.
	if status, _ := call("PATCH", "/v1/admin/vendors/"+vendorID, admin, `{"is_active":false}`); status != http.StatusOK {
		t.Fatalf("deactivate vendor: %d", status)
	}
	if status, _ := call("GET", "/v1/vendor/me", bearer(vendorSession), ""); status != http.StatusUnauthorized {
		t.Errorf("vendor session after deactivation: got %d, want 401", status)
	}

	// Admin logout kills the admin session.
	if status, _ := call("POST", "/v1/admin/auth/logout", admin, ""); status != http.StatusNoContent {
		t.Errorf("admin logout: got %d", status)
	}
	if status, _ := call("GET", "/v1/admin/me", admin, ""); status != http.StatusUnauthorized {
		t.Errorf("admin session after logout: got %d, want 401", status)
	}
}
