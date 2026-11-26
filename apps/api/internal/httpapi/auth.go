package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/shyun-cc/half-km/apps/api/internal/dbtx"
)

type principal struct {
	Role  string `json:"role"`
	ID    int64  `json:"id"`
	Buyer string `json:"buyer_id"`
}

func (p principal) store(id int64) error {
	if (p.Role == "operator" || p.ID == id) && id > 0 {
		return nil
	}
	return dbtx.ErrDenied
}

type Auth struct {
	tokens    map[[sha256.Size]byte]principal
	pickupKey []byte
}

func LoadAuth(path string) (Auth, error) {
	if path == "" {
		return Auth{}, nil
	}
	var config struct {
		PickupKey string `json:"pickup_key_hex"`
		Tokens    []struct {
			Digest string `json:"token_sha256"`
			principal
		} `json:"tokens"`
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Auth{}, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return Auth{}, errors.New("invalid authorization configuration")
	}
	key, err := hex.DecodeString(config.PickupKey)
	if err != nil || len(key) != 32 || len(config.Tokens) == 0 {
		return Auth{}, errors.New("authorization needs a 32-byte pickup key and at least one token digest")
	}
	auth := Auth{tokens: make(map[[sha256.Size]byte]principal), pickupKey: key}
	for _, entry := range config.Tokens {
		digest, err := hex.DecodeString(entry.Digest)
		if err != nil || len(digest) != sha256.Size {
			return Auth{}, errors.New("invalid token digest")
		}
		p := entry.principal
		switch p.Role {
		case "operator":
			if p.ID != 0 || p.Buyer != "" {
				return Auth{}, errors.New("invalid operator principal")
			}
		case "buyer":
			id, err := uuid.Parse(p.Buyer)
			if err != nil || id == uuid.Nil || p.ID != 0 {
				return Auth{}, errors.New("invalid buyer principal")
			}
			p.Buyer = id.String()
		case "merchant", "payments", "leader":
			if p.ID <= 0 || p.Buyer != "" {
				return Auth{}, errors.New("invalid scoped principal")
			}
		default:
			return Auth{}, errors.New("unknown authorization role")
		}
		key := [sha256.Size]byte(digest)
		if _, found := auth.tokens[key]; found {
			return Auth{}, errors.New("duplicate token digest")
		}
		auth.tokens[key] = p
	}
	return auth, nil
}

func (a Auth) authenticate(header string) (principal, bool) {
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || len(token) < 32 || len(token) > 256 {
		return principal{}, false
	}
	p, ok := a.tokens[sha256.Sum256([]byte(token))]
	return p, ok
}
