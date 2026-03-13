CREATE TABLE IF NOT EXISTS notes (
  note_id TEXT PRIMARY KEY,
  contact_id TEXT NOT NULL,
  body TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_notes_contact_created_at ON notes(contact_id, created_at);
