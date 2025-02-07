package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {
    token := jwt.NewWithClaims(
        jwt.SigningMethodHS256,
        jwt.RegisteredClaims{
            Issuer: "chirpy",
            IssuedAt: jwt.NewNumericDate(time.Now().UTC()),
            ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(expiresIn)),
            Subject: userID.String(),
        },
    )

    tokenString, err := token.SignedString(([]byte)(tokenSecret))
    if err != nil {
        fmt.Printf("\nError at MakeJWT: %v\n", err)
        return "", fmt.Errorf("error while creating jwt")
    }

    return tokenString, nil
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
    token, err := jwt.ParseWithClaims(
        tokenString,
        &jwt.RegisteredClaims{},
        func(token *jwt.Token) (interface{}, error) {
	        return []byte(tokenSecret), nil
        },
    )

    if err != nil {
        if err.Error() == "token has invalid claims: token is expired" {
            return uuid.UUID{}, fmt.Errorf("Expired Token")
        }
        return uuid.UUID{}, fmt.Errorf("Invalid Token")
    }

    id, err := token.Claims.GetSubject()
    if err != nil {
        return uuid.UUID{}, fmt.Errorf("Error on Token's Claims")
    }

    return uuid.MustParse(id), nil
}
