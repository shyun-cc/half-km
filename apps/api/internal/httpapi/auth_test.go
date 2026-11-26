package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestLoadAuth(t *testing.T) {
	token := hex.EncodeToString(randBytes(t, 32))
	digest := sha256.Sum256([]byte(token))
	buyer := uuid.NewString()
	data, err := json.Marshal(map[string]any{
		"pickup_key_hex": hex.EncodeToString(randBytes(t, 32)),
		"tokens":         []map[string]string{{"token_sha256": hex.EncodeToString(digest[:]), "role": "buyer", "buyer_id": buyer}},
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	auth, err := LoadAuth(path)
	if err != nil {
		t.Fatal(err)
	}
	if p, ok := auth.authenticate("Bearer " + token); !ok || p.Role != "buyer" || p.Buyer != buyer {
		t.Fatalf("unexpected principal: %+v", p)
	}
	if _, ok := auth.authenticate("Bearer " + hex.EncodeToString(digest[:])); ok {
		t.Fatal("token digest accepted as a credential")
	}
}
