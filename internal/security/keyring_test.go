package security

import (
	"bytes"
	"encoding/base64"
	"errors"
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
