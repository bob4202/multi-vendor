// Package push sends Web Push notifications: the payload is encrypted for
// the browser (RFC 8291, aes128gcm) and the request is signed with the
// server's VAPID key (RFC 8292). Standard library only.
package push

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrGone means the browser unsubscribed (404/410): delete the subscription.
var ErrGone = errors.New("push subscription is no longer valid")

// Subscription is what the browser's PushManager.subscribe() returns.
type Subscription struct {
	Endpoint string
	P256dh   string // base64url, the browser's public key
	Auth     string // base64url, 16-byte auth secret
}

// Message is what the service worker receives and shows.
type Message struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url,omitempty"`
	Tag   string `json:"tag,omitempty"` // a newer message with the same tag replaces the old one
}

type Sender struct {
	publicKey  string // base64url, uncompressed P-256 point
	privateKey *ecdsa.PrivateKey
	subject    string
	client     *http.Client
	now        func() time.Time
}

// New returns nil, nil when push isn't configured (both keys empty).
func New(publicKey, privateKey, subject string) (*Sender, error) {
	if publicKey == "" && privateKey == "" {
		return nil, nil
	}
	raw, err := decodeB64(privateKey)
	if err != nil {
		return nil, fmt.Errorf("VAPID_PRIVATE_KEY: %w", err)
	}
	key, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), raw)
	if err != nil {
		return nil, fmt.Errorf("VAPID_PRIVATE_KEY: %w", err)
	}
	pub, err := key.PublicKey.Bytes()
	if err != nil {
		return nil, err
	}
	if base64.RawURLEncoding.EncodeToString(pub) != strings.TrimRight(publicKey, "=") {
		return nil, errors.New("VAPID_PUBLIC_KEY doesn't match VAPID_PRIVATE_KEY")
	}
	return &Sender{
		publicKey:  base64.RawURLEncoding.EncodeToString(pub),
		privateKey: key,
		subject:    subject,
		client:     &http.Client{Timeout: 15 * time.Second},
		now:        time.Now,
	}, nil
}

// PublicKey is the applicationServerKey the browser subscribes with.
func (s *Sender) PublicKey() string {
	if s == nil {
		return ""
	}
	return s.publicKey
}

// GenerateKeys makes a new VAPID key pair (base64url), for cmd/vapid.
func GenerateKeys() (publicKey, privateKey string, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	pub, err := key.PublicKey.Bytes()
	if err != nil {
		return "", "", err
	}
	priv, err := key.Bytes()
	if err != nil {
		return "", "", err
	}
	return base64.RawURLEncoding.EncodeToString(pub), base64.RawURLEncoding.EncodeToString(priv), nil
}

// Send delivers one message to one browser.
func (s *Sender) Send(ctx context.Context, sub Subscription, msg Message) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	body, err := encrypt(sub, payload)
	if err != nil {
		return err
	}
	endpoint, err := url.Parse(sub.Endpoint)
	if err != nil || endpoint.Scheme != "https" {
		return fmt.Errorf("%w: endpoint must be https", ErrGone)
	}
	jwt, err := s.vapidJWT(endpoint.Scheme + "://" + endpoint.Host)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.Endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("TTL", "86400")
	req.Header.Set("Urgency", "high")
	req.Header.Set("Authorization", fmt.Sprintf("vapid t=%s, k=%s", jwt, s.publicKey))

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return ErrGone
	case resp.StatusCode >= 300:
		return fmt.Errorf("push service answered %s", resp.Status)
	}
	return nil
}

// vapidJWT signs {aud, exp, sub} with ES256, as the push service expects.
func (s *Sender) vapidJWT(audience string) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"typ":"JWT","alg":"ES256"}`))
	claims, err := json.Marshal(map[string]any{
		"aud": audience,
		"exp": s.now().Add(12 * time.Hour).Unix(),
		"sub": s.subject,
	})
	if err != nil {
		return "", err
	}
	unsigned := header + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(unsigned))
	r, sig, err := ecdsa.Sign(rand.Reader, s.privateKey, digest[:])
	if err != nil {
		return "", err
	}
	// JWS wants r || s, each left-padded to 32 bytes.
	raw := make([]byte, 64)
	r.FillBytes(raw[:32])
	sig.FillBytes(raw[32:])
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(raw), nil
}

const recordSize = 4096

// encrypt implements RFC 8291 (Message Encryption for Web Push) with the
// aes128gcm content coding from RFC 8188, as a single record.
func encrypt(sub Subscription, plaintext []byte) ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	asPrivate, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return encryptWith(sub, plaintext, salt, asPrivate)
}

// encryptWith takes the random parts as arguments so the RFC's test vector
// can be reproduced exactly.
func encryptWith(sub Subscription, plaintext, salt []byte, asPrivate *ecdh.PrivateKey) ([]byte, error) {
	uaPublicRaw, err := decodeB64(sub.P256dh)
	if err != nil {
		return nil, fmt.Errorf("%w: bad p256dh", ErrGone)
	}
	authSecret, err := decodeB64(sub.Auth)
	if err != nil || len(authSecret) != 16 {
		return nil, fmt.Errorf("%w: bad auth secret", ErrGone)
	}
	uaPublic, err := ecdh.P256().NewPublicKey(uaPublicRaw)
	if err != nil {
		return nil, fmt.Errorf("%w: bad p256dh", ErrGone)
	}

	asPublic := asPrivate.PublicKey().Bytes()
	ecdhSecret, err := asPrivate.ECDH(uaPublic)
	if err != nil {
		return nil, err
	}

	// IKM = HKDF(auth_secret, ecdh_secret, "WebPush: info" || 0 || ua_public || as_public, 32)
	prkKey, err := hkdf.Extract(sha256.New, ecdhSecret, authSecret)
	if err != nil {
		return nil, err
	}
	keyInfo := "WebPush: info\x00" + string(uaPublicRaw) + string(asPublic)
	ikm, err := hkdf.Expand(sha256.New, prkKey, keyInfo, 32)
	if err != nil {
		return nil, err
	}

	prk, err := hkdf.Extract(sha256.New, ikm, salt)
	if err != nil {
		return nil, err
	}
	cek, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, err
	}
	nonce, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	// One record: the plaintext, then the 0x02 "last record" delimiter.
	padded := append(append([]byte{}, plaintext...), 0x02)
	if len(padded)+gcm.Overhead() > recordSize {
		return nil, errors.New("push payload too large")
	}
	ciphertext := gcm.Seal(nil, nonce, padded, nil)

	// Header: salt (16) | record size (4) | key id length (1) | key id (as_public)
	out := make([]byte, 0, 16+4+1+len(asPublic)+len(ciphertext))
	out = append(out, salt...)
	out = binary.BigEndian.AppendUint32(out, recordSize)
	out = append(out, byte(len(asPublic)))
	out = append(out, asPublic...)
	out = append(out, ciphertext...)
	return out, nil
}

// decodeB64 accepts base64url with or without padding (browsers send
// unpadded; some libraries pad).
func decodeB64(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
}
