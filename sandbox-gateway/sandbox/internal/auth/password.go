package auth

import (
	"golang.org/x/crypto/bcrypt"
)

// HashPassword returns a bcrypt hash of the plaintext password (startup / NewGuard).
func HashPassword(plain string) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
}

// PasswordMatch compares a bcrypt hash (from HashPassword / NewGuard) with a
// candidate password using bcrypt.CompareHashAndPassword (CodeQL #21/#22).
func PasswordMatch(hashed []byte, given string) bool {
	if len(hashed) == 0 {
		return false
	}
	return bcrypt.CompareHashAndPassword(hashed, []byte(given)) == nil
}
