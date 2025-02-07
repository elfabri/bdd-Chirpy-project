package auth

import (
	"fmt"
	"net/http"
	"testing"
)

func TestBearer(t *testing.T) {
    validT := "ThisIsSuperSecretTokenToTellWeAreIndeedInYourBase"
    valid := "Bearer " + validT
    invalid := "Bea " + validT
    countFail := 0
    countPass := 0

    fmt.Print("testing valid bearer token...\n")
    header, err := http.Head("http://example.com")
    if err != nil {
        t.Errorf("Bad header: %v\n", err)
        return
    }
    http.Header.Set(header.Header, "Authorization", valid)
    tok, err := GetBearerToken(header.Header)
    if err != nil || tok != validT {
        countFail++
        t.Errorf("The valid token did NOT pass: %v\ntoken: %s", err, tok)
    } else {
        countPass++
    }
    
    http.Header.Set(header.Header, "Authorization", invalid)
    tok, err = GetBearerToken(header.Header)
    if err != nil {
        countPass++
    } else {
        countFail++
        t.Errorf("The invalid token DID pass: %s\n", tok)
    }

    fmt.Printf("Test Finished:\n\t + PASSED: %d\n\t - FAILED: %d\n", countPass, countFail)
}

