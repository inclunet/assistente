CREATE TABLE conversations (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT,
  channel TEXT,
  contact_id TEXT,
  summary TEXT,
  summary_up_to_message_id INTEGER,
  summarizing_in_progress INTEGER DEFAULT 0,
  created_at DATETIME,
  updated_at DATETIME
);

CREATE TABLE chat_messages (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  conversation_id INTEGER,
  parent_id INTEGER,
  turn_id INTEGER,
  role TEXT,
  content TEXT,
  reasoning TEXT,
  media TEXT,
  audio TEXT,
  audio_mime_type TEXT,
  tool_calls TEXT,
  tool_call_id TEXT,
  prompt_tokens INTEGER DEFAULT 0,
  completion_tokens INTEGER DEFAULT 0,
  total_tokens INTEGER DEFAULT 0,
  model TEXT,
  source TEXT,
  created_at DATETIME,
  updated_at DATETIME
);

INSERT INTO conversations
  (id, title, created_at, updated_at)
VALUES
  (7, 'Fixture 0.1.9 sem PII', '2026-03-18T11:59:00Z', '2026-03-18T12:00:00Z');

INSERT INTO chat_messages
  (id, conversation_id, role, content, created_at, updated_at)
VALUES
  (41, 7, 'user', 'mensagem de teste', '2026-03-18T11:59:01Z', '2026-03-18T11:59:01Z');

INSERT INTO chat_messages
  (id, conversation_id, parent_id, turn_id, role, content, created_at, updated_at)
VALUES
  (42, 7, 41, 41, 'assistant', 'resposta de teste', '2026-03-18T11:59:02Z', '2026-03-18T11:59:02Z');
