ALTER TABLE contacts ADD COLUMN signing_public_key TEXT NOT NULL DEFAULT '';
ALTER TABLE contacts ADD COLUMN relay_url TEXT NOT NULL DEFAULT '';
ALTER TABLE contacts ADD COLUMN recipient_id TEXT NOT NULL DEFAULT '';
ALTER TABLE contacts ADD COLUMN raw_identity_card_json TEXT NOT NULL DEFAULT '';
