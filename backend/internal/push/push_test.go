package push

import (
	"crypto/ecdh"
	"encoding/base64"
	"strings"
	"testing"
)

// RFC 8291 Appendix A.
func TestEncryptMatchesRFC8291Vector(t *testing.T) {
	b64 := func(s string) []byte {
		t.Helper()
		v, err := base64.RawURLEncoding.DecodeString(s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	asPrivate, err := ecdh.P256().NewPrivateKey(b64("yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"))
	if err != nil {
		t.Fatal(err)
	}
	sub := Subscription{
		P256dh: "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4",
		Auth:   "BTBZMqHH6r4Tts7J_aSIgg",
	}
	got, err := encryptWith(sub, []byte("When I grow up, I want to be a watermelon"), b64("DGv6ra1nlYgDCS1FRnbzlw"), asPrivate)
	if err != nil {
		t.Fatal(err)
	}
	want := "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPTpK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN"
	if g := base64.RawURLEncoding.EncodeToString(got); g != want {
		t.Fatalf("ciphertext mismatch\n got  %s\n want %s", g, want)
	}
}

func TestKeysRoundTripAndJWT(t *testing.T) {
	pub, priv, err := GenerateKeys()
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(pub, priv, "mailto:test@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if s.PublicKey() != pub {
		t.Fatalf("public key = %q, want %q", s.PublicKey(), pub)
	}
	jwt, err := s.vapidJWT("https://push.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if parts := strings.Split(jwt, "."); len(parts) != 3 || len(b64len(parts[2])) != 64 {
		t.Fatalf("unexpected jwt %q", jwt)
	}
	if _, err := New(pub, "", "x"); err == nil {
		t.Fatal("expected error for half-configured keys")
	}
}

func b64len(s string) []byte {
	v, _ := base64.RawURLEncoding.DecodeString(s)
	return v
}
