package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/elfabri/bdd-Chirpy-project/internal/auth"
	"github.com/elfabri/bdd-Chirpy-project/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

// stateful handlers
type apiConfig struct {
    // atomic for safely increment
    // and read an integer value
    // across multiple goroutines (HTTP requests)
	fileserverHits atomic.Int32
    dbQueries *database.Queries
    platform string
    secret string
}

type errors struct {
    Error string `json:"error"`
}

// middleware to count views
func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {

    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        cfg.fileserverHits.Add(1)
        next.ServeHTTP(w, r)
    })
}

// readiness handler
func readiness(resW http.ResponseWriter, req *http.Request) {
    req.Header.Set("Content-Type", "application/json")
    resW.WriteHeader(200)
    resW.Write([]byte("OK"))
}

func cleanChirp(badChirp string, profaneW []string) string {
    words := strings.Split(badChirp, " ")
    for i, w := range words {
        for _, pw := range profaneW {
            if lower_w := strings.ToLower(w); lower_w == pw {
                words[i] = "****"
            }
        }
    }
    return strings.Join(words, " ")
}

type chirpError struct {
    e error
    num uint8
}

// validate length and censor profane words
func validate_chirp(sus_chirp string) (string, chirpError) {
    profaneW := []string{"kerfuffle", "sharbert", "fornax"}

    if sus_chirp == "" {
        return "", chirpError{nil, 1}
    }

    if len(sus_chirp) > 140 {
        return "", chirpError{nil, 2}
    }

    // filter profane words
    cleanedBdy := ""
    lower_body := strings.ToLower(sus_chirp)
    for _, pw := range profaneW {
        if strings.Contains(lower_body, pw) {
            // clean the profanity
            cleanedBdy = cleanChirp(sus_chirp, profaneW)
            break
        }
    }

    if cleanedBdy == "" {
        // no profane words founded
        return sus_chirp, chirpError{nil, 0}
    }

    return cleanedBdy, chirpError{nil, 0}

}

// view count handler
func (cfg *apiConfig) views(w http.ResponseWriter, r *http.Request) {
    r.Header.Set("Content-Type", "text/html")
    content := fmt.Sprintf(`
    <html>
    <body>
        <h1>Welcome, Chirpy Admin</h1>
        <p>Chirpy has been visited %d times!</p>
    </body>
    </html>
    `, cfg.fileserverHits.Load())

    w.Write([]byte(content))
}

// view count reset
func (cfg *apiConfig) reset(w http.ResponseWriter, r *http.Request) {
    cfg.fileserverHits.Store(0)
    if cfg.platform != "dev" {
        http.Error(w, "Forbidden", http.StatusForbidden)
        return
    }

    err := cfg.dbQueries.DeleteAllUsers(r.Context())
    if err != nil {
        http.Error(w, "Internal Server Error", http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusOK)
}

// create user handler
func (cfg *apiConfig) create_user(w http.ResponseWriter, r *http.Request) {
    r.Header.Set("Content-Type", "application/json")

    type parameters struct{
        Email string `json:"email"`
        Password string `json:"password"`
    }

    decoder := json.NewDecoder(r.Body)
    params := parameters{}
    err := decoder.Decode(&params)
    if err != nil {
        log.Printf("Error creating user: %v\n", err)
    }

    if params.Email == "" {
        log.Printf("invalid User email: %s\n", params.Email)
    }

    hashedPassw, err := auth.HahsPassword(params.Password)
    if err != nil {
        log.Printf("error hashing the user's password: %s\n", params.Email)
    }
    userParams := database.CreateUserParams{
        Email: params.Email,
        HashedPassword: hashedPassw,
    }

    user := database.User{}
    user, err = cfg.dbQueries.CreateUser(r.Context(), userParams)
    if err != nil {
        log.Printf("Error creating user in db: %v\n", err)
    }

    type userRes struct {
        Id string `json:"id"`
        CreatedAt string `json:"created_at"`
        UpdatedAt string `json:"updated_at"`
        Email string `json:"email"`
    }

    userR := userRes {
        Id: user.ID.String(),
        CreatedAt: user.CreatedAt.String(),
        UpdatedAt: user.UpdatedAt.String(),
        Email: user.Email,
    }

    w.WriteHeader(201)
    encodedUserRes, err := json.Marshal(userR)
    if err != nil {
        log.Printf("error marshalling user response: %v", err)
    }
    w.Write(encodedUserRes)
}

// login handler
func (cfg *apiConfig) login_user(w http.ResponseWriter, r *http.Request) {
    r.Header.Set("Content-Type", "application/json")

    type parameters struct{
        Email string `json:"email"`
        Password string `json:"password"`
    }

    decoder := json.NewDecoder(r.Body)
    params := parameters{}
    err := decoder.Decode(&params)
    if err != nil {
        log.Printf("Error while user's login: %v\n", err)
    }

    // user lookup
    user, err := cfg.dbQueries.ShowUserByEmail(r.Context(), params.Email)
    if err != nil {
        log.Printf("Error while searching user with email: %s, error %v\n", params.Email, err)
        w.WriteHeader(401)
        respError := errors{
            Error: "Incorrect email or password",
        }
        encodedError, err := json.Marshal(respError)
        if err != nil {
            log.Printf("Error encoding Error JSON: %s", err)
            return
        }
        w.Write(encodedError)
        return
    }

    // passw comparison
    err = auth.CheckPasswordHash(user.HashedPassword, params.Password)
    if err != nil {
        log.Printf("Error while authenticating user with email: %s, error %v\n", params.Email, err)
        w.WriteHeader(401)
        respError := errors{
            Error: "Incorrect email or password",
        }
        encodedError, err := json.Marshal(respError)
        if err != nil {
            log.Printf("Error encoding Error JSON: %s", err)
            return
        }
        w.Write(encodedError)
        return
    }

    // token gen for authentication
    token, err := auth.MakeJWT(
        user.ID,
        cfg.secret,
        time.Hour,
    )
    if err != nil {
        log.Printf("Generation of jwt token failed: %s\n", err)
        return
    }

    // refresh token gen
    r_token, err := auth.MakeRefreshToken()
    if err != nil {
        log.Printf("Generation of refresh token failed: %s\n", err)
        return
    }

    // refresh_token stored in db, expires in 60 days
    userRTParams := database.InsertRTokenParams{
        Token: r_token,
        UserID: user.ID,
        ExpiresAt: time.Now().Add(time.Hour * 24 * 60),

    }

    _, err = cfg.dbQueries.InsertRToken(r.Context(),userRTParams)
    if err != nil {
        log.Printf("Error while inserting refresh token into db: %s\n", err)
        return
    }

    type userRes struct {
        Id string `json:"id"`
        CreatedAt string `json:"created_at"`
        UpdatedAt string `json:"updated_at"`
        Email string `json:"email"`
        Token string `json:"token"`
        Ref_Token string `json:"refresh_token"`
    }

    userR := userRes {
        Id: user.ID.String(),
        CreatedAt: user.CreatedAt.String(),
        UpdatedAt: user.UpdatedAt.String(),
        Email: user.Email,
        Token: token,
        Ref_Token: r_token,
    }

    w.WriteHeader(200)
    encodedUserRes, err := json.Marshal(userR)
    if err != nil {
        log.Printf("error marshalling user response: %v", err)
    }
    w.Write(encodedUserRes)
}

// check refresh token from db
// does not accept a request body, but does require
// a refresh token to be present in the headers
// return a jwt token
func (cfg *apiConfig) check_ref_tok(w http.ResponseWriter, r *http.Request) {
    rtok, err := auth.GetBearerToken(r.Header)
    if err != nil {
        log.Printf("error while getting user token: %v\n", err)
        w.WriteHeader(401)
        return
    }

    // search the token in db refresh_token
    // check existance, expireDate and if it was revoked
    rT, err := cfg.dbQueries.GetUserFromRToken(r.Context(), rtok)
    if err != nil {
        log.Printf("Error while getting user with refresh_token, error: %v\n", err)
        w.WriteHeader(401)
        return
    }
    if rT.ExpiresAt.Before(time.Now()) {
        log.Printf("Token has already expired\n")
        w.WriteHeader(401)
        return
    }
    if rT.RevokedAt.Valid {
        log.Printf("Token has been revoked\n")
        w.WriteHeader(401)
        return
    }

    // token gen for authentication
    jwt, err := auth.MakeJWT(
        rT.UserID,
        cfg.secret,
        time.Hour,
    )

    if err != nil {
        log.Printf("Generation of jwt token failed: %s\n", err)
        return
    }
    type validRToken struct {
        Token string `json:"token"`
    }

    valid := validRToken{
        Token: jwt,
    }
    
    encodedValidRes, err := json.Marshal(valid)
    if err != nil {
        log.Printf("error marshalling refresh token: %v\n", err)
        return
    }
    w.WriteHeader(200)
    w.Write(encodedValidRes)
}

// revoke refresh token
func (cfg *apiConfig) revoke_ref_tok(w http.ResponseWriter, r *http.Request) {
    tok, err := auth.GetBearerToken(r.Header)
    if err != nil {
        log.Printf("error while getting user token: %v\n", err)
        return
    }
    err = cfg.dbQueries.RevokeRToken(r.Context(), tok)
    if err != nil {
        log.Printf("error while revoking user token: %v\n", err)
        return 
    }
    w.WriteHeader(204)
}

// create chirp
func (cfg *apiConfig) create_chirp(w http.ResponseWriter, r *http.Request) {
    r.Header.Set("Content-Type", "application/json")

    token, err := auth.GetBearerToken(r.Header)
    if err != nil {
        log.Printf("Error getting token from bearer: %s", err)
        w.WriteHeader(401)
        respError := errors {
            Error: "Something went wrong",
        }
        encodedError, err := json.Marshal(respError)
        if err != nil {
            fmt.Printf("Error encoding Error JSON: %s", err)
            return
        }
        w.Write(encodedError)
        return
    }

    type chirpRequest struct {
        Body   string    `json:"body"`
        UserID string `json:"user_id"`
    }

    decoder := json.NewDecoder(r.Body)
    params := chirpRequest{}
    err = decoder.Decode(&params)

    if err != nil {
        log.Printf("Error decoding parameters: %s", err)
        w.WriteHeader(500)
        respError := errors{
            Error: "Something went wrong",
        }
        encodedError, err := json.Marshal(respError)
        if err != nil {
            fmt.Printf("Error encoding Error JSON: %s", err)
            return
        }
        w.Write(encodedError)
        return
    }

    // user verification with jwt
    userID, err := auth.ValidateJWT(token, cfg.secret)
    if err != nil {
        log.Printf("Error validating token from user: %s", err)
        w.WriteHeader(401)
        respError := errors {
            Error: "Something went wrong",
        }
        encodedError, err := json.Marshal(respError)
        if err != nil {
            fmt.Printf("Error encoding Error JSON: %s", err)
            return
        }
        w.Write(encodedError)
        return
    }

    // validate chirp
    validChirp, chirpError := validate_chirp(params.Body)
    if chirpError.num != 0 {
        switch chirpError.num {
        case 1:
            // nil chirp error
            log.Println("error, chirp can not be null")
            w.WriteHeader(400)
            respError := errors {
                Error: "Chirp is null",
            }
            encodedError, err := json.Marshal(respError)
            if err != nil {
                fmt.Printf("Error encoding Error JSON: %s", err)
                return
            }
            w.Write(encodedError)
            return
        case 2:
            // too long (>140) chirp error
            w.WriteHeader(400)
            respError := errors{
                Error: "Chirp is too long",
            }
            encodedError, err := json.Marshal(respError)
            if err != nil {
                fmt.Printf("Error encoding Error JSON: %s", err)
                return
            }
            w.Write(encodedError)
            return
        }
    }

    params.Body = validChirp
    chirp := database.Chirp{}
    chirp, err = cfg.dbQueries.CreateChirp(
        r.Context(),
        database.CreateChirpParams{
            Body: params.Body,
            UserID: userID,
        },
    )
    if err != nil {
        log.Printf("Error creating chirp in db: %v\n", err)
    }

    chirpData, err := json.Marshal(chirp)
    if err != nil {
        log.Printf("Error marshalling chirp data: %v\n", err)
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(201)
    w.Write(chirpData)
}

// show all chirps ordered by created_at
func (cfg *apiConfig) show_chirps(w http.ResponseWriter, r *http.Request) {

    chirps, err := cfg.dbQueries.ShowChirp( r.Context() )

    chirpData, err := json.Marshal(chirps)
    if err != nil {
        log.Printf("Error marshalling chirp data: %v\n", err)
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(200)
    w.Write(chirpData)
}

// show specific chirp
func (cfg *apiConfig) show_chirp_by_id(w http.ResponseWriter, r *http.Request) {
    chirpID := r.PathValue("chirpID")

    chirpUUID, err := uuid.Parse(chirpID)
    if err != nil {
        log.Printf("Error parsing chirp uuid: %s;\n - error: %v\n", chirpID,  err)
    }

    chirp, err := cfg.dbQueries.ShowChirpByID( r.Context(), chirpUUID )
    chirpData, err := json.Marshal(chirp)
    if err != nil {
        log.Printf("Error marshalling chirp data: %v\n", err)
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(200)
    w.Write(chirpData)
}

func main() {
    godotenv.Load()
    dbURL := os.Getenv("DB_URL")
    db, err := sql.Open("postgres", dbURL)
    if err != nil {
        log.Fatal("Error loading .env file")
    }

    mux := http.NewServeMux()
    server := &http.Server{
        Addr:       ":8080",
        Handler:    mux,
    }

    dbQueries := database.New(db)
    apiCfg := apiConfig {
        fileserverHits: atomic.Int32{},
        dbQueries: dbQueries,
        platform: os.Getenv("PLATFORM"),
        secret: os.Getenv("SECRET"),
    }

    // handler main page
    handler := http.StripPrefix("/app/", http.FileServer(http.Dir(".")))
    mux.Handle("/app/", apiCfg.middlewareMetricsInc(handler))

    // readiness endpoint
    mux.HandleFunc("GET /api/healthz", readiness)

    // view couter show & reset
    mux.HandleFunc("GET /admin/metrics", apiCfg.views)
    mux.HandleFunc("POST /admin/reset", apiCfg.reset)

    mux.Handle("/assets", http.FileServer(http.Dir("./assets/logo.png")))

    // create users
    mux.HandleFunc("POST /api/users", apiCfg.create_user)

    // login user
    mux.HandleFunc("POST /api/login", apiCfg.login_user)

    // refresh_token lookup
    mux.HandleFunc("POST /api/refresh", apiCfg.check_ref_tok)

    // revoke refresh_token
    mux.HandleFunc("POST /api/revoke", apiCfg.revoke_ref_tok)

    // create chirps
    mux.HandleFunc("POST /api/chirps", apiCfg.create_chirp)

    // show all chirps
    mux.HandleFunc("GET /api/chirps", apiCfg.show_chirps)

    // show specific chirp by id
    mux.HandleFunc("GET /api/chirps/{chirpID}", apiCfg.show_chirp_by_id)

    if err := server.ListenAndServe(); err != nil {
        fmt.Printf("error: %v", err)
    }
}
