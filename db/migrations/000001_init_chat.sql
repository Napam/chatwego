-- +goose Up
CREATE TABLE chat_sessions (
  id TEXT PRIMARY KEY,
  title TEXT,
  metadata TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
) STRICT;

/*
===========================================================================
TABLE: chat_messages
===========================================================================
A "multi-role" table: system, user, assistant, and tool rows live together so
the whole conversation comes back from one ordered query in the shape
OpenAI-compatible chat APIs expect, with no JOINs.

`type` selects the role:
- system    -- hidden instructions in `content`.
- user      -- the prompt in `content`.
- assistant -- reply text in `content` (often NULL when the round only calls
               tools), requested calls as JSON in `tool_calls`, thinking
               output in `reasoning`.
- tool      -- the result in `content`, with `tool_call_id` linking it back
               to the assistant call that requested it.

Unused columns are NULL by convention; the schema does not enforce that.
===========================================================================
*/
CREATE TABLE chat_messages (
  id INTEGER PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES chat_sessions(id) ON UPDATE CASCADE ON DELETE CASCADE,
  type TEXT NOT NULL CHECK (type IN ('system', 'user', 'assistant', 'tool')),
  content TEXT,
  reasoning TEXT,
  tool_calls TEXT,
  tool_call_id TEXT,
  tokens_used INTEGER,
  metadata TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
) STRICT;

CREATE INDEX idx_chat_messages_session ON chat_messages (session_id, created_at);

-- Bump chat_sessions.updated_at whenever a new message is inserted
-- +goose StatementBegin
CREATE TRIGGER trg_chat_messages_update_session AFTER INSERT ON chat_messages FOR EACH ROW BEGIN
  UPDATE chat_sessions SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.session_id;
END;
-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS chat_messages;

DROP TABLE IF EXISTS chat_sessions;
