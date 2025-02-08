package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strings"

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

func GetBearerToken(headers http.Header) (string, error) {
    info := headers.Get("Authorization")
    if info == "" {
        return "", fmt.Errorf("Couldn't get Authorization")
    }

    infoParts := strings.Split(info, " ")
    if infoParts[0] != "Bearer" {
        return "", fmt.Errorf("Couldn't get Authorization")
    }

    return infoParts[1], nil
}

func MakeRefreshToken() (string, error) {
    randData := make([]byte, 32)
    _, err := rand.Read(randData)
    if err != nil {
        return "", fmt.Errorf("Error while creating random data for refresh token: %v", err)
    }

    token := hex.EncodeToString(randData)
    return token, nil
}
