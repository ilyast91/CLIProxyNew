package security

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestHashAndVerifySecret(t *testing.T) {
	t.Parallel()

	const secret = "client-api-key"

	hash, err := HashSecret(secret)
	if err != nil {
		t.Fatalf("HashSecret() error = %v", err)
	}
	if hash == secret {
		t.Fatal("HashSecret() вернул исходный секрет")
	}
	if !VerifySecret(hash, secret) {
		t.Fatal("VerifySecret() не принял правильный секрет")
	}
	if VerifySecret(hash, "wrong-secret") {
		t.Fatal("VerifySecret() принял неправильный секрет")
	}
}

func TestHashSecretUsesConfiguredCost(t *testing.T) {
	t.Parallel()

	hash, err := HashSecret("client-api-key")
	if err != nil {
		t.Fatalf("HashSecret() error = %v", err)
	}

	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil {
		t.Fatalf("bcrypt.Cost() error = %v", err)
	}
	if cost != 12 {
		t.Fatalf("bcrypt.Cost() = %d, want 12", cost)
	}
}
