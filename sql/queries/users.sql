-- name: CreateUser :one
INSERT INTO users (id, created_at, updated_at, email, hashed_password)
VALUES (
    gen_random_uuid(),
    NOW(),
    NOW(),
    $1,
    $2
)
RETURNING *;

-- name: ShowUserByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: UpdateUser :exec
UPDATE users
SET email = $2, updated_at = NOW(), hashed_password = $3
WHERE id = $1;

-- name: UpgradeUser :exec
UPDATE users
SET is_chirpy_red = true, updated_at = NOW()
WHERE id = $1;

