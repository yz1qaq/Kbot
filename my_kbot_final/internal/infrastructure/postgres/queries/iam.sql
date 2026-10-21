-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1 LIMIT 1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1 LIMIT 1;

-- name: CreateUser :one
INSERT INTO users (
    id, email, password_hash, name, role, status, created_at, updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, now(), now())
RETURNING *;

-- name: ListUsers :many
SELECT * FROM users
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: SetUserRole :exec
UPDATE users
SET role = $2, updated_at = now()
WHERE id = $1;