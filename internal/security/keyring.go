package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

// AES256KeySize — размер мастер-ключа AES-256 в байтах.
const AES256KeySize = 32

var (
	// ErrInvalidKey означает некорректный формат или размер мастер-ключа.
	ErrInvalidKey = errors.New("некорректный мастер-ключ")
	// ErrUnknownKeyVersion означает отсутствие ключа нужной версии.
	ErrUnknownKeyVersion = errors.New("неизвестная версия мастер-ключа")
	// ErrInvalidCiphertext означает повреждённый или подменённый шифротекст.
	ErrInvalidCiphertext = errors.New("некорректный шифротекст")
)

// EncryptedValue соответствует паре credentials_enc и enc_key_version в БД.
type EncryptedValue struct {
	KeyVersion int
	Ciphertext []byte
}

// Keyring хранит активный мастер-ключ и предыдущие версии для ротации.
type Keyring struct {
	activeVersion int
	keys          map[int][]byte
}

// NewKeyring создаёт keyring и копирует переданные ключи.
func NewKeyring(activeVersion int, keys map[int][]byte) (*Keyring, error) {
	if activeVersion <= 0 {
		return nil, fmt.Errorf("%w: активная версия должна быть положительной", ErrInvalidKey)
	}
	if _, ok := keys[activeVersion]; !ok {
		return nil, fmt.Errorf("%w: активная версия %d", ErrUnknownKeyVersion, activeVersion)
	}

	keyCopies := make(map[int][]byte, len(keys))
	for version, key := range keys {
		if version <= 0 || len(key) != AES256KeySize {
			return nil, fmt.Errorf("%w: версия %d должна содержать %d байт", ErrInvalidKey, version, AES256KeySize)
		}
		keyCopies[version] = append([]byte(nil), key...)
	}

	return &Keyring{activeVersion: activeVersion, keys: keyCopies}, nil
}

// ParseBase64Key разбирает base64-значение CLIPROXY_ENCRYPTION_KEY.
func ParseBase64Key(encoded string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return nil, fmt.Errorf("%w: base64: %v", ErrInvalidKey, err)
	}
	if len(key) != AES256KeySize {
		return nil, fmt.Errorf("%w: получено %d байт, требуется %d", ErrInvalidKey, len(key), AES256KeySize)
	}
	return key, nil
}

// Encrypt шифрует данные активным ключом AES-256-GCM.
func (k *Keyring) Encrypt(plaintext []byte) (EncryptedValue, error) {
	gcm, err := k.gcm(k.activeVersion)
	if err != nil {
		return EncryptedValue{}, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return EncryptedValue{}, fmt.Errorf("создать nonce: %w", err)
	}

	aad := keyVersionAAD(k.activeVersion)
	ciphertext := gcm.Seal(nonce, nonce, plaintext, aad)
	return EncryptedValue{KeyVersion: k.activeVersion, Ciphertext: ciphertext}, nil
}

// Decrypt расшифровывает данные ключом указанной версии.
func (k *Keyring) Decrypt(value EncryptedValue) ([]byte, error) {
	gcm, err := k.gcm(value.KeyVersion)
	if err != nil {
		return nil, err
	}
	if len(value.Ciphertext) < gcm.NonceSize()+gcm.Overhead() {
		return nil, ErrInvalidCiphertext
	}

	nonce := value.Ciphertext[:gcm.NonceSize()]
	ciphertext := value.Ciphertext[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, keyVersionAAD(value.KeyVersion))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidCiphertext, err)
	}
	return plaintext, nil
}

func (k *Keyring) gcm(version int) (cipher.AEAD, error) {
	key, ok := k.keys[version]
	if !ok {
		return nil, fmt.Errorf("%w: %d", ErrUnknownKeyVersion, version)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("создать AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("создать GCM: %w", err)
	}
	return gcm, nil
}

func keyVersionAAD(version int) []byte {
	aad := make([]byte, 8)
	binary.BigEndian.PutUint64(aad, uint64(version))
	return aad
}
