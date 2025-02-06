package auth

import (
	"log"

	"golang.org/x/crypto/bcrypt"
)

func HahsPassword(passw string) (string, error) {
    hashedP, err := bcrypt.GenerateFromPassword(([]byte)(passw), 5)
    if err != nil {
        log.Printf("Error hashing the password: %v", err)
    }

    return (string)(hashedP), nil
}

func CheckPasswordHash(password, hash string) error {
    return bcrypt.CompareHashAndPassword(([]byte)(password), ([]byte)(hash))
}
