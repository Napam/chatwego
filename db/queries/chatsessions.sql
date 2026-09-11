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

-- name: CreateChatMessage :one
INSERT INTO
  chat_messages (session_id, type, content, reasoning, tool_calls, tool_call_id, created_at)
VALUES
  (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
RETURNING
  *;

-- name: GetMessagesBySession :many
-- Order by id (the monotonic rowid), not created_at: CURRENT_TIMESTAMP has
-- one-second resolution, so every message written during one reply shares a
-- timestamp and would come back in an undefined order.
SELECT
  *
FROM
  chat_messages
WHERE
  session_id = ?
ORDER BY
  id ASC;
