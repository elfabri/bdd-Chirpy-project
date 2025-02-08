-- name: InsertRToken :one
INSERT INTO refresh_tokens (token, created_at, updated_at, user_id, expires_at, revoked_at)
VALUES (
    $1,
    NOW(),
    NOW(),
    $2,
    $3,
    NULL
)
RETURNING *;

-- name: GetUserFromRToken :one
SELECT * FROM refresh_tokens WHERE token = $1;

-- name: RefreshRToken :exec
UPDATE refresh_tokens
SET token = $2, updated_at = NOW(), expires_at = $3
WHERE user_id = $1;

-- name: RevokeRToken :exec
UPDATE refresh_tokens
SET updated_at = NOW(), revoked_at = NOW()
WHERE token = $1;

