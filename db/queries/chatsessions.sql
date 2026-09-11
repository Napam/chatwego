-- name: GetSessionsByOwner :many
SELECT
  *
FROM
  chat_sessions
WHERE
  identity_id = ?
ORDER BY
  updated_at DESC;

-- name: GetSessionById :one
SELECT
  *
FROM
  chat_sessions
WHERE
  id = ?;

-- name: CreateChatSession :one
INSERT INTO
  chat_sessions (id, title, identity_id, created_at, updated_at)
VALUES
  (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
RETURNING
  *;

-- name: UpdateChatSessionTitle :exec
UPDATE
  chat_sessions
SET
  title = ?,
  updated_at = CURRENT_TIMESTAMP
WHERE
  id = ?;

-- name: CreateChatMessage :one
INSERT INTO
  chat_messages (session_id, type, content, reasoning, tool_calls, tool_call_id, created_at)
VALUES
  (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
RETURNING
  *;

-- name: GetMessagesBySession :many
SELECT
  *
FROM
  chat_messages
WHERE
  session_id = ?
ORDER BY
  created_at ASC;
