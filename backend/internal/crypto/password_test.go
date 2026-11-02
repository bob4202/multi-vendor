package crypto

import (
	"strings"
	"testing"
)

func TestPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Errorf("unexpected hash format: %s", hash)
	}

	ok, err := VerifyPassword("correct horse battery staple", hash)
	if err != nil || !ok {
		t.Errorf("expected correct password to verify, got ok=%v err=%v", ok, err)
	}

	ok, err = VerifyPassword("wrong password", hash)
	if err != nil || ok {
		t.Errorf("expected wrong password to fail, got ok=%v err=%v", ok, err)
	}
}

func TestHashPasswordUsesFreshSalt(t *testing.T) {
	a, _ := HashPassword("same")
	b, _ := HashPassword("same")
	if a == b {
		t.Error("two hashes of the same password must differ (random salt)")
	}
}

func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	for _, bad := range []string{"", "plaintext", "$argon2i$v=19$m=1,t=1,p=1$c2FsdA$aGFzaA", "$argon2id$v=19$m=x$c2FsdA$aGFzaA"} {
		if _, err := VerifyPassword("x", bad); err == nil {
			t.Errorf("expected error for malformed hash %q", bad)
		}
	}
}

func TestNeedsRehash(t *testing.T) {
	current, _ := HashPassword("pw")
	if NeedsRehash(current) {
		t.Error("a hash made with current params should not need a rehash")
	}
	// Same format, weaker parameters.
	old := strings.Replace(current, "m=19456,t=2,p=1", "m=4096,t=1,p=1", 1)
	if !NeedsRehash(old) {
		t.Error("a hash made with different params should need a rehash")
	}
}

func TestGenerateTokenPrefix(t *testing.T) {
	tok, err := GenerateToken(SessionTokenPrefix)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.HasPrefix(tok, SessionTokenPrefix) || len(tok) != len(SessionTokenPrefix)+64 {
		t.Errorf("unexpected token %q", tok)
	}
}
