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
    "sort"

	"github.com/elfabri/bdd-Chirpy-project/internal/auth"
	"github.com/elfabri/bdd-Chirpy-project/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

// bdd test
var cachedChirpID uuid.UUID

// stateful handlers
type apiConfig struct {
    // atomic for safely increment
    // and read an integer value
    // across multiple goroutines (HTTP requests)
	fileserverHits atomic.Int32
    dbQueries *database.Queries
    platform string
    secret string
    polka_key string
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
        IsChirpyRed bool `json:"is_chirpy_red"`
    }

    userR := userRes {
        Id: user.ID.String(),
        CreatedAt: user.CreatedAt.String(),
        UpdatedAt: user.UpdatedAt.String(),
        Email: user.Email,
        IsChirpyRed: user.IsChirpyRed,
    }

    w.WriteHeader(201)
    encodedUserRes, err := json.Marshal(userR)
    if err != nil {
        log.Printf("error marshalling user response: %v", err)
    }
    w.Write(encodedUserRes)
}

// update user's email and password
// requires access token in the header
// new passw and email in the body
func (cfg *apiConfig) update_user(w http.ResponseWriter, r *http.Request) {
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

    // user verification with jwt
    userID, err := auth.ValidateJWT(token, cfg.secret)
    if err != nil {
        log.Printf("Error, invalid refresh token: %s\n", err)
        w.WriteHeader(401)
        respError := errors {
            Error: "Something went wrong",
        }
        encodedError, err := json.Marshal(respError)
        if err != nil {
            fmt.Printf("Error encoding Error JSON: %s\n", err)
            return
        }
        w.Write(encodedError)
        return
    }

    // get new email and passw from request body
    type parameters struct{
        Email string `json:"email"`
        Password string `json:"password"`
    }

    decoder := json.NewDecoder(r.Body)
    params := parameters{}
    err = decoder.Decode(&params)
    if err != nil {
        log.Printf("Error decoding user's login info: %v\n", err)
        w.WriteHeader(401)
        return
    }

    hashedPassw, err := auth.HahsPassword(params.Password)
    if err != nil {
        log.Printf("error hashing the user's password: %s\n", params.Email)
        w.WriteHeader(401)
        return
    }

    updateUserParams := database.UpdateUserParams {
        ID: userID,
        Email: params.Email,
        HashedPassword: hashedPassw,
    }

    err = cfg.dbQueries.UpdateUser(r.Context(), updateUserParams)
    if err != nil {
        log.Printf("Error updating user in db: %v\n", err)
        w.WriteHeader(401)
        return
    }

    // get user with userID
    userUpdated, err := cfg.dbQueries.GetUserByID(r.Context(), userID)
    if err != nil {
        log.Printf("Error, couldn't find user after update: %s\n", err)
        w.WriteHeader(401)
        respError := errors {
            Error: "Something went wrong",
        }
        encodedError, err := json.Marshal(respError)
        if err != nil {
            fmt.Printf("Error encoding Error JSON: %s\n", err)
            return
        }
        w.Write(encodedError)
        return
    }

    // return user updated
    type userRes struct {
        Id string `json:"id"`
        CreatedAt string `json:"created_at"`
        UpdatedAt string `json:"updated_at"`
        Email string `json:"email"`
    }

    userR := userRes {
        Id: userUpdated.ID.String(),
        CreatedAt: userUpdated.CreatedAt.String(),
        UpdatedAt: userUpdated.UpdatedAt.String(),
        Email: userUpdated.Email,
    }

    encodedUserRes, err := json.Marshal(userR)
    if err != nil {
        log.Printf("error marshalling user response: %v", err)
    }
    w.Write(encodedUserRes)
    w.WriteHeader(200)
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
    user, err := cfg.dbQueries.GetUserByEmail(r.Context(), params.Email)
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
        IsChirpyRed bool `json:"is_chirpy_red"`
    }

    userR := userRes {
        Id: user.ID.String(),
        CreatedAt: user.CreatedAt.String(),
        UpdatedAt: user.UpdatedAt.String(),
        Email: user.Email,
        Token: token,
        Ref_Token: r_token,
        IsChirpyRed: user.IsChirpyRed,
    }

    encodedUserRes, err := json.Marshal(userR)
    if err != nil {
        log.Printf("error marshalling user response: %v\n", err)
        w.WriteHeader(500)
        return
    }
    w.WriteHeader(200)
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

    // cache chirpID to be use on bdd tests
    cachedChirpID = chirp.ID

    chirpData, err := json.Marshal(chirp)
    if err != nil {
        log.Printf("Error marshalling chirp data: %v\n", err)
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(201)
    w.Write(chirpData)
}

// get all chirps ordered by created_at
func (cfg *apiConfig) get_chirps(w http.ResponseWriter, r *http.Request) {
    // optional query "author_id"
    author_id := r.URL.Query().Get("author_id")
    // optional query "sort"
    order := r.URL.Query().Get("sort")
    var chirps []database.Chirp
    var err error
    if author_id != "" {
        user_id, err := uuid.Parse(author_id)
        if err != nil {
            log.Printf("Invalid author_id: %v\n", err)
            w.WriteHeader(404)
            return
        }

        chirps, err = cfg.dbQueries.GetChirpsFromUser( r.Context(), user_id )
        if err != nil {
            log.Printf("author_id not found: %v\n", err)
            w.WriteHeader(404)
            return
        }

    } else {
        chirps, err = cfg.dbQueries.GetChirps( r.Context() )
        if err != nil {
            log.Printf("Error while getting chirps: %v\n", err)
            w.WriteHeader(404)
            return
        }
    }

    // sort asc is default
    if order == "desc" {
        sort.Slice(chirps, func(i, j int) bool {
            return chirps[i].CreatedAt.After(chirps[j].CreatedAt)
        })
    }

    chirpData, err := json.Marshal(chirps)
    if err != nil {
        log.Printf("Error marshalling chirp data: %v\n", err)
        w.WriteHeader(500)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(200)
    w.Write(chirpData)
}

// get specific chirp searched by chirp_id
func (cfg *apiConfig) get_chirp_by_id(w http.ResponseWriter, r *http.Request) {
    chirpID := r.PathValue("chirpID")

    if chirpID[0:2] == "${" && os.Getenv("PLATFORM") == "dev" {
        // test chirp should be ${chirpID} format
        log.Printf("Parsing test chirp: %s\n", chirpID)
        chirpID = cachedChirpID.String()
    }

    chirpUUID, err := uuid.Parse(chirpID)
    if err != nil {
        log.Printf("Error parsing chirp uuid: %s;\n - error: %v\n", chirpID,  err)
        w.WriteHeader(500)
        return
    }

    chirp, err := cfg.dbQueries.GetChirpByID( r.Context(), chirpUUID )
    if err != nil {
        log.Printf("Error searching for chirp: %v\n", err)
        w.WriteHeader(404)
        return
    }

    chirpData, err := json.Marshal(chirp)
    if err != nil {
        log.Printf("Error marshalling chirp data: %v\n", err)
        w.WriteHeader(500)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(200)
    w.Write(chirpData)
}

// delete specific chirp
// can only delete users' own chirps
// user's validation on req.header
func (cfg *apiConfig) delete_chirp_by_id(w http.ResponseWriter, r *http.Request) {
    r.Header.Set("Content-Type", "application/json")

    token, err := auth.GetBearerToken(r.Header)
    if err != nil {
        log.Printf("Error getting token from bearer: %s", err)
        w.WriteHeader(401)
        return
    }

    // user verification with jwt
    userID, err := auth.ValidateJWT(token, cfg.secret)
    if err != nil {
        log.Printf("Error validating token from user: %s", err)
        w.WriteHeader(401)
        return
    }

    chirpID := r.PathValue("chirpID")

    var chirpUUID uuid.UUID
    // chirp Id may be a test of format "${chirpID}"
    if chirpID[0:2] == "${" && os.Getenv("PLATFORM") == "dev" {
        // test chirp should be ${chirpID} format
        log.Printf("Using cached test chirp: %s\n", chirpID)
        chirpUUID = cachedChirpID
    } else {
        chirpUUID, err = uuid.Parse(chirpID)
        if err != nil {
            log.Printf("Error parsing chirp uuid: %s;\n - error: %v\n", chirpID,  err)
            w.WriteHeader(500)
            return
        }
    }

    chirp, err := cfg.dbQueries.GetChirpByID( r.Context(), chirpUUID )
    if err != nil {
        log.Printf("Error searching for chirp: %v\n", err)
        w.WriteHeader(404)
        return
    }

    if chirp.UserID != userID {
        log.Print("Invalid chirp deletion\n")
        log.Print("Trying to delete someone else's chirp\n")
        w.WriteHeader(403)
        return
    }

    err = cfg.dbQueries.DeleteChirp( r.Context(), chirpUUID )
    if err != nil {
        log.Printf("Error deleting chirp: %v\n", err)
        w.WriteHeader(500)
        return
    }

    w.WriteHeader(204)
}

// polka handler - upgrade user to chirpy red
func (cfg *apiConfig) upgrade_user(w http.ResponseWriter, r *http.Request) {
    r.Header.Set("Content-Type", "application/json")

    apiKey, err := auth.GetAPIKey(r.Header)
    if err != nil {
        log.Printf("error while getting api key: %v\n", err)
        w.WriteHeader(401)
        return
    }

    if apiKey != cfg.polka_key {
        log.Print("Wrong Api Key to upgrade user\n")
        w.WriteHeader(401)
        return
    }

    type dataParams struct {
        UserID string `json:"user_id"`
    }

    type upgradeParams struct {
        Event   string  `json:"event"`
        Data    dataParams  `json:"data"`
    }

    decoder := json.NewDecoder(r.Body)
    params := upgradeParams{}
    err = decoder.Decode(&params)
    if err != nil {
        log.Printf("Error while decoding params to upgrade user: %v\n", err)
        w.WriteHeader(500)
        return
    }

    if params.Event != "user.upgraded" {
        w.WriteHeader(204)
        return
    }

    id, err := uuid.Parse(params.Data.UserID)
    if err != nil {
        log.Print("Invalid User ID\n")
        log.Printf("Couldn't parse user id (%v) while upgrading to chirpy red: %v\n", id, err)
        w.WriteHeader(500)
        return
    }

    err = cfg.dbQueries.UpgradeUser(r.Context(), id)
    if err != nil {
        log.Print("User Not Found\n")
        log.Printf("Couldn't upgrade user (id: %v) to chirpy red: %v\n", id, err)
        w.WriteHeader(404)
        return
    }
    log.Printf(" - - Upgraded user (id: %v) to chirpy red\n", id)
    w.WriteHeader(204)
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
        polka_key: os.Getenv("POLKA_KEY"),
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

    // update users emails and/or passwords
    mux.HandleFunc("PUT /api/users", apiCfg.update_user)

    // login user
    mux.HandleFunc("POST /api/login", apiCfg.login_user)

    // refresh_token lookup
    mux.HandleFunc("POST /api/refresh", apiCfg.check_ref_tok)

    // revoke refresh_token
    mux.HandleFunc("POST /api/revoke", apiCfg.revoke_ref_tok)

    // create chirps
    mux.HandleFunc("POST /api/chirps", apiCfg.create_chirp)

    // get all chirps
    mux.HandleFunc("GET /api/chirps", apiCfg.get_chirps)

    // get specific chirp by id
    mux.HandleFunc("GET /api/chirps/{chirpID}", apiCfg.get_chirp_by_id)

    // delete specific chirp by id
    mux.HandleFunc("DELETE /api/chirps/{chirpID}", apiCfg.delete_chirp_by_id)

    // upgrade user to chirpy red
    mux.HandleFunc("POST /api/polka/webhooks", apiCfg.upgrade_user)

    if err := server.ListenAndServe(); err != nil {
        fmt.Printf("error: %v", err)
    }
}
