// Command admin does platform chores that need no running API:
//
//	admin create --email EMAIL --name NAME
//	    Create a platform admin (the people who sign in to /admin). The
//	    first admin has to be made here; there is deliberately no sign-up
//	    page. The password is read from stdin (one line), so it never
//	    appears in shell history or the process list - `make admin` prompts
//	    for it without echoing.
//
//	admin storefront-key --host HOST
//	    Issue a new storefront key for the branch with that inventory host
//	    and print it as frontend/.env lines (BRANCH_ID, STOREFRONT_KEY).
//	    Replaces any existing key, so a storefront still using the old one
//	    stops working. `make setup` uses it for local development.
//
// Both need DATABASE_URL.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ecommerce/backend/internal/crypto"
	"ecommerce/backend/internal/db"
	"ecommerce/backend/internal/platform"
)

const usage = `usage:
  admin create --email EMAIL --name NAME   (password on stdin)
  admin storefront-key --host HOST`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL is required")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()

	switch args[0] {
	case "create":
		return createAdmin(ctx, pool, args[1:])
	case "storefront-key":
		return storefrontKey(ctx, pool, args[1:])
	default:
		return errors.New(usage)
	}
}

func createAdmin(ctx context.Context, pool *pgxpool.Pool, args []string) error {
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	email := fs.String("email", "", "admin email (login)")
	name := fs.String("name", "", "admin display name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *email == "" || *name == "" {
		return errors.New("--email and --name are required")
	}

	// Prompt only when typing interactively; `make admin` pipes the
	// password in after its own no-echo prompt.
	interactive := false
	if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
		interactive = true
		fmt.Fprint(os.Stderr, "Password (min 12 characters): ")
	}
	password, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && password == "" {
		return fmt.Errorf("read password: %w", err)
	}
	password = strings.TrimRight(password, "\r\n")
	if interactive {
		fmt.Fprintln(os.Stderr)
	}

	admin, err := platform.NewService(pool).CreateAdmin(ctx, *email, *name, password)
	if err != nil {
		return err
	}
	fmt.Printf("Created platform admin %s <%s>\n", admin.Name, admin.Email)
	return nil
}

func storefrontKey(ctx context.Context, pool *pgxpool.Pool, args []string) error {
	fs := flag.NewFlagSet("storefront-key", flag.ContinueOnError)
	host := fs.String("host", "", "the branch's inventory system host")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *host == "" {
		return errors.New("--host is required")
	}

	var branchID uuid.UUID
	err := pool.QueryRow(ctx, `SELECT id FROM branches WHERE host = $1 ORDER BY created_at LIMIT 1`, *host).Scan(&branchID)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("no branch with host %q (run make seed first?)", *host)
	}
	if err != nil {
		return err
	}

	key, err := crypto.GenerateAPIKey()
	if err != nil {
		return err
	}
	prefix := crypto.KeyPrefix(key)
	if _, err := db.New(pool).SetBranchStorefrontKey(ctx, db.SetBranchStorefrontKeyParams{
		ID:                     branchID,
		StorefrontApiKeyHash:   crypto.HashAPIKey(key),
		StorefrontApiKeyPrefix: &prefix,
	}); err != nil {
		return fmt.Errorf("store key: %w", err)
	}

	// stdout is exactly the .env lines, so scripts can capture it.
	fmt.Printf("BRANCH_ID=%s\nSTOREFRONT_KEY=%s\n", branchID, key)
	return nil
}
