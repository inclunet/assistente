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

CREATE TABLE editor_documents (
  id TEXT PRIMARY KEY,
  title TEXT DEFAULT '',
  mode TEXT DEFAULT 'markdown',
  file_path TEXT,
  markdown TEXT,
  created_at DATETIME,
  updated_at DATETIME
);

CREATE TABLE editor_session_states (
  id TEXT PRIMARY KEY,
  json TEXT,
  created_at DATETIME,
  updated_at DATETIME
);

INSERT INTO editor_documents
  (id, title, mode, markdown, created_at, updated_at)
VALUES
  ('rascunho-publicado', 'Rascunho sintético', 'markdown', '# Conteúdo sintético 0.1.9', '2026-03-18T11:58:00Z', '2026-03-18T11:59:00Z');

INSERT INTO editor_documents
  (id, title, mode, markdown, created_at, updated_at)
VALUES
  ('rascunho-conflito', 'Conflito sintético', 'markdown', 'versão sintética local', '2026-03-18T11:58:10Z', '2026-03-18T11:59:10Z');

INSERT INTO editor_session_states
  (id, json, created_at, updated_at)
VALUES
  (
    'default',
    '{"version":2,"autoSaveEnabled":false,"activeTabId":"tab-publicada","profileSlug":"editor-texto","tabs":[{"id":"tab-publicada","title":"Documento sintético","mode":"rich","draftId":"rascunho-publicado"}],"fileModeByPath":{"documento-sintetico.md":"rich"},"externalConflictLockedByTabId":{"tab-publicada":true},"mergeSessionsByTabId":{"tab-publicada":{"originalPath":"documento-sintetico.md","mineDraftId":"rascunho-publicado","diskDraftId":"rascunho-conflito","conflictDraftId":"rascunho-conflito","createdAt":1773835140000}}}',
    '2026-03-18T11:58:20Z',
    '2026-03-18T11:59:20Z'
  );
