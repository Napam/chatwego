-- name: GetIdentityById :one
SELECT
  *
FROM
  identities
WHERE
  id = ?;

-- name: GetLoginBySourceExternalId :one
SELECT
  *
FROM
  logins
WHERE
  source = ?
  AND external_id = ?;

-- name: CreateLogin :one
INSERT INTO
  logins (id, identity_id, external_id, source, raw_source_data, linked_at)
VALUES
  (?, ?, ?, ?, ?, ?)
RETURNING
  *;

-- name: GetSessionTokenByHash :one
SELECT
  *
FROM
  session_tokens
WHERE
  hash = ?
  AND revoked_at IS NULL;

-- name: CreateSessionToken :one
INSERT INTO
  session_tokens (hash, login_id, identity_id, last_used_at, expires_at, created_at)
VALUES
  (?, ?, ?, ?, ?, ?)
RETURNING
  *;

-- name: UpdateSessionTokenLastUsed :exec
UPDATE
  session_tokens
SET
  last_used_at = ?
WHERE
  hash = ?;

-- name: RevokeSessionToken :exec
UPDATE
  session_tokens
SET
  revoked_at = ?
WHERE
  hash = ?;
