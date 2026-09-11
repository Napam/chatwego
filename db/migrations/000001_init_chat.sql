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
This is a "Multi-role table". LLM APIs (like OpenAI or Anthropic) require
the entire conversation history to be passed as a single, chronological array
of messages. By keeping all roles in one table, a simple ORDER BY created_at
gives us the exact payload the API expects without complex JOINs.

Column usage depends strictly on the 'role':

1. role = 'system'
- Purpose: System prompts or hidden instructions (e.g., "You are an agent").
- Uses: `content` holds the instructions.
- Nulls: `tool_calls` and `tool_call_id` must be NULL.

2. role = 'user'
- Purpose: The human's actual input.
- Uses: `content` holds the user's prompt.
- Nulls: `tool_calls` and `tool_call_id` must be NULL.

3. role = 'assistant'
- Purpose: The LLM's response. It can either speak OR call a tool.
- Uses:
- If speaking: `content` holds the text response.
- If calling a tool: `content` is often NULL, and `tool_calls`
holds a JSON array of the requested functions.
- `reasoning` optionally holds the model's reasoning/thinking
output for this round, if the model exposes any.
- Nulls: `tool_call_id` must be NULL.

4. role = 'tool'
- Purpose: Your application returning the result of a tool execution.
- Uses: `content` holds the raw result (text or JSON from the API).
`tool_call_id` holds the unique string linking this result
back to the assistant's original request.
- Nulls: `tool_calls` must be NULL.
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
