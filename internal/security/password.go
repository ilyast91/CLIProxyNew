package security

import "golang.org/x/crypto/bcrypt"

const bcryptCost = 12

// HashSecret создаёт bcrypt-хэш секрета с зафиксированной стоимостью R5.
func HashSecret(secret string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// VerifySecret проверяет секрет по bcrypt-хэшу.
func VerifySecret(hash, secret string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(secret)) == nil
}
