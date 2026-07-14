package access

import (
	"fmt"
	"strconv"
	"strings"
)

const principalPrefix = "cliproxy:v1:"

// Principal связывает запрос с пользователем и клиентским API-ключом.
type Principal struct {
	UserID   int64
	APIKeyID *int64
}

// EncodePrincipal возвращает versioned principal для передачи через SDK usage record.
func EncodePrincipal(userID, apiKeyID int64) string {
	return fmt.Sprintf("%s%d:%d", principalPrefix, userID, apiKeyID)
}

// DecodePrincipal разбирает versioned principal и legacy user ID.
func DecodePrincipal(value string) (Principal, error) {
	if strings.HasPrefix(value, principalPrefix) {
		parts := strings.Split(strings.TrimPrefix(value, principalPrefix), ":")
		if len(parts) != 2 {
			return Principal{}, fmt.Errorf("invalid principal")
		}
		userID, err := positiveID(parts[0])
		if err != nil {
			return Principal{}, err
		}
		apiKeyID, err := positiveID(parts[1])
		if err != nil {
			return Principal{}, err
		}
		return Principal{UserID: userID, APIKeyID: &apiKeyID}, nil
	}

	userID, err := positiveID(value)
	if err != nil {
		return Principal{}, err
	}
	return Principal{UserID: userID}, nil
}

func positiveID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid principal")
	}
	return id, nil
}
