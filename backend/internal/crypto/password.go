package crypto

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// argon2id parameters - OWASP's recommended minimum (19 MiB, 2 iterations,
// 1 thread). They're encoded into every hash, so raising them later only
// affects new hashes; NeedsRehash lets login upgrade old ones transparently.
const (
	argonMemoryKiB  = 19 * 1024
	argonIterations = 2
	argonThreads    = 1
	argonSaltLen    = 16
	argonKeyLen     = 32
)

var ErrInvalidPasswordHash = errors.New("invalid password hash")

type argonParams struct {
	memory     uint32
	iterations uint32
	threads    uint8
}

var currentArgonParams = argonParams{argonMemoryKiB, argonIterations, argonThreads}

// HashPassword returns an argon2id hash in the standard PHC string format:
// $argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash>
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	p := currentArgonParams
	key := argon2.IDKey([]byte(password), salt, p.iterations, p.memory, p.threads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.memory, p.iterations, p.threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword reports whether password matches encoded, comparing in
// constant time. A malformed hash is an error, not a mismatch, so a corrupt
// row surfaces instead of silently locking someone out.
func VerifyPassword(password, encoded string) (bool, error) {
	p, salt, want, err := decodeArgonHash(encoded)
	if err != nil {
		return false, err
	}
	got := argon2.IDKey([]byte(password), salt, p.iterations, p.memory, p.threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// NeedsRehash reports whether encoded was made with different parameters
// than the current ones, so it should be replaced after a successful login.
func NeedsRehash(encoded string) bool {
	p, _, _, err := decodeArgonHash(encoded)
	return err != nil || p != currentArgonParams
}

func decodeArgonHash(encoded string) (argonParams, []byte, []byte, error) {
	// "", "argon2id", "v=19", "m=...,t=...,p=...", salt, hash
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return argonParams{}, nil, nil, ErrInvalidPasswordHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return argonParams{}, nil, nil, ErrInvalidPasswordHash
	}

	var p argonParams
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.iterations, &p.threads); err != nil {
		return argonParams{}, nil, nil, ErrInvalidPasswordHash
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return argonParams{}, nil, nil, ErrInvalidPasswordHash
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(key) == 0 {
		return argonParams{}, nil, nil, ErrInvalidPasswordHash
	}
	return p, salt, key, nil
}
