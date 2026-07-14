package security

import (
	"bytes"
	"encoding/base64"
	"errors"
	"math"
	"testing"
)

func TestKeyringEncryptDecrypt(t *testing.T) {
	t.Parallel()

	keyring, err := NewKeyring(2, map[int][]byte{
		1: bytes.Repeat([]byte{1}, AES256KeySize),
		2: bytes.Repeat([]byte{2}, AES256KeySize),
	})
	if err != nil {
		t.Fatalf("NewKeyring() error = %v", err)
	}

	plaintext := []byte(`{"access_token":"secret"}`)
	value, err := keyring.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if value.KeyVersion != 2 {
		t.Fatalf("Encrypt() key version = %d, want 2", value.KeyVersion)
	}
	if bytes.Contains(value.Ciphertext, plaintext) {
		t.Fatal("Encrypt() оставил plaintext в шифротексте")
	}

	got, err := keyring.Decrypt(value)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("Decrypt() = %q, want %q", got, plaintext)
	}
}

func TestKeyringRejectsUnknownVersion(t *testing.T) {
	t.Parallel()

	keyring, err := NewKeyring(1, map[int][]byte{1: bytes.Repeat([]byte{1}, AES256KeySize)})
	if err != nil {
		t.Fatalf("NewKeyring() error = %v", err)
	}

	_, err = keyring.Decrypt(EncryptedValue{KeyVersion: 9, Ciphertext: []byte("ciphertext")})
	if !errors.Is(err, ErrUnknownKeyVersion) {
		t.Fatalf("Decrypt() error = %v, want ErrUnknownKeyVersion", err)
	}
}

func TestKeyringRejectsTamperedCiphertext(t *testing.T) {
	t.Parallel()

	keyring, err := NewKeyring(1, map[int][]byte{1: bytes.Repeat([]byte{1}, AES256KeySize)})
	if err != nil {
		t.Fatalf("NewKeyring() error = %v", err)
	}

	value, err := keyring.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	value.Ciphertext[len(value.Ciphertext)-1] ^= 0xff

	_, err = keyring.Decrypt(value)
	if !errors.Is(err, ErrInvalidCiphertext) {
		t.Fatalf("Decrypt() error = %v, want ErrInvalidCiphertext", err)
	}
}

func TestParseBase64Key(t *testing.T) {
	t.Parallel()

	want := bytes.Repeat([]byte{7}, AES256KeySize)
	got, err := ParseBase64Key(base64.StdEncoding.EncodeToString(want))
	if err != nil {
		t.Fatalf("ParseBase64Key() error = %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("ParseBase64Key() вернул другой ключ")
	}

	if _, err := ParseBase64Key(base64.StdEncoding.EncodeToString([]byte("short"))); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("ParseBase64Key(short) error = %v, want ErrInvalidKey", err)
	}
	if _, err := ParseBase64Key("not-base64"); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("ParseBase64Key(invalid) error = %v, want ErrInvalidKey", err)
	}
}

func TestNewKeyringFromBase64LoadsPreviousVersions(t *testing.T) {
	t.Parallel()

	keyV1 := bytes.Repeat([]byte{1}, AES256KeySize)
	keyV2 := bytes.Repeat([]byte{2}, AES256KeySize)
	oldKeyring, err := NewKeyring(1, map[int][]byte{1: keyV1})
	if err != nil {
		t.Fatalf("NewKeyring(v1) error = %v", err)
	}
	oldValue, err := oldKeyring.Encrypt([]byte("old credential"))
	if err != nil {
		t.Fatalf("Encrypt(v1) error = %v", err)
	}

	keyring, err := NewKeyringFromBase64(
		2,
		base64.StdEncoding.EncodeToString(keyV2),
		`{"1":"`+base64.StdEncoding.EncodeToString(keyV1)+`"}`,
	)
	if err != nil {
		t.Fatalf("NewKeyringFromBase64() error = %v", err)
	}
	got, err := keyring.Decrypt(oldValue)
	if err != nil {
		t.Fatalf("Decrypt(previous version) error = %v", err)
	}
	if string(got) != "old credential" {
		t.Fatalf("Decrypt(previous version) = %q", got)
	}

	newValue, err := keyring.Encrypt([]byte("new credential"))
	if err != nil {
		t.Fatalf("Encrypt(active version) error = %v", err)
	}
	if newValue.KeyVersion != 2 {
		t.Fatalf("Encrypt(active version) = %d, want 2", newValue.KeyVersion)
	}
}

func TestNewKeyringFromBase64RejectsInvalidPreviousKeys(t *testing.T) {
	t.Parallel()

	active := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, AES256KeySize))
	tests := []string{
		`{"2":"` + active + `"}`,
		`{"not-a-version":"` + active + `"}`,
		`{"1":"short"}`,
	}
	for _, previous := range tests {
		if _, err := NewKeyringFromBase64(2, active, previous); !errors.Is(err, ErrInvalidKey) {
			t.Fatalf("NewKeyringFromBase64(%q) error = %v, want ErrInvalidKey", previous, err)
		}
	}
}

func TestNewKeyringFromBase64RejectsDatabaseVersionOverflow(t *testing.T) {
	t.Parallel()

	active := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, AES256KeySize))
	if _, err := NewKeyringFromBase64(int(math.MaxInt32)+1, active, ""); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("NewKeyringFromBase64(overflow) error = %v, want ErrInvalidKey", err)
	}
}
