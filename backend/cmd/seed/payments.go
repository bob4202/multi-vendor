package main

import (
	"context"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"ecommerce/backend/internal/crypto"
)

// The fake provider's keys. Nothing checks them except the fake itself, so
// fixed values are fine and let the local Pay / Fail page sign its webhooks
// (X-Fake-Secret) without looking anything up.
const (
	fakeSecretKey     = "fake_sk_test_local"
	fakeWebhookSecret = "fake_whsec_local"
)

// seedPaymentAccount gives the vendor an active fake payment account, so
// checkout works locally. Skips vendors that already have an active account
// (e.g. real Stripe test keys connected through the vendor portal).
func seedPaymentAccount(ctx context.Context, pool *pgxpool.Pool, vendorID uuid.UUID) (bool, error) {
	box, err := crypto.NewSecretBox(os.Getenv("APP_ENCRYPTION_KEY"))
	if err != nil {
		return false, err
	}
	secretEnc, err := box.Encrypt(fakeSecretKey)
	if err != nil {
		return false, err
	}
	webhookEnc, err := box.Encrypt(fakeWebhookSecret)
	if err != nil {
		return false, err
	}

	tag, err := pool.Exec(ctx, `
		INSERT INTO vendor_payment_accounts (
			vendor_id, provider, live, display_name, secret_key_enc, secret_key_last4,
			webhook_id, webhook_secret_enc, status
		)
		SELECT $1, 'fake', false, 'Fake Test Shop', $2, $3, 'fake_webhook', $4, 'active'
		WHERE NOT EXISTS (
			SELECT 1 FROM vendor_payment_accounts WHERE vendor_id = $1 AND status = 'active'
		)`,
		vendorID, secretEnc, fakeSecretKey[len(fakeSecretKey)-4:], webhookEnc,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
