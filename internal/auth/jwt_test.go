package auth

/*
Make sure that you can create and validate JWTs,
and that expired tokens are rejected
and JWTs signed with the wrong secret are rejected
*/

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestJWT(t *testing.T) {
    type testCase struct {
        userID          uuid.UUID
        expireDuration  time.Duration
    }

    userID1, _ := uuid.NewRandom()
    userID2, _ := uuid.NewRandom()

    tests := []testCase {
        {
            userID: userID1,
            expireDuration: time.Duration(time.Second * 5),
        },
        {
            userID: userID2,
            expireDuration: time.Duration(time.Second * 3),
        },
    }

    passCount := 0
    failCount := 0
    tokenSecret := os.Getenv("SECRET")

    fmt.Print("\t-\tRunning validation test\n")
    for _, test := range tests {
        jwtToken, err := MakeJWT(test.userID, tokenSecret, test.expireDuration)
        if err != nil {
            failCount++
            fmt.Println("----------------------")
            t.Errorf("Couldn't make jwt token: %v", err)
            fmt.Println("----------------------")
            continue
        }
        passCount++

        valid, err := ValidateJWT(jwtToken, tokenSecret)
        if err != nil {
            failCount++
            fmt.Println("----------------------")
            t.Errorf("ERROR:\n")
            t.Errorf("couldn't validate user with id: %v\n", test.userID)
            t.Errorf("%v", err)
            fmt.Println("----------------------")
            continue
        }
        passCount++

        if valid != test.userID {
            failCount++
            fmt.Println("----------------------")
            t.Errorf("ERROR:\n")
            t.Errorf("User id: %v != validated user id: %v", test.userID, valid)
            fmt.Println("----------------------")
            continue
        }
        passCount++

        fmt.Printf("--> \tAwaiting Expiration of tokens\n")
        time.Sleep(test.expireDuration + time.Second * 3)
        fmt.Printf("> \tTesting Expired token\n")
        invalid, err2 := ValidateJWT(jwtToken, tokenSecret)
        // err2 expected to be Expired Token
        if err2.Error() != "Expired Token" {
            failCount++
            fmt.Println("----------------------")
            t.Errorf("ERROR:\n")
            t.Errorf("Should have been invalid but got: %v", invalid)
            fmt.Println("----------------------")
            continue
        }
        passCount++
    }

    fmt.Println("---------------------------------")
	fmt.Printf("%d passed, %d failed\n", passCount, failCount)
}
