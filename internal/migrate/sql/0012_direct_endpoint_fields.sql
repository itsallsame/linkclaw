ALTER TABLE self_messaging_profiles ADD COLUMN direct_url TEXT NOT NULL DEFAULT '';
ALTER TABLE self_messaging_profiles ADD COLUMN direct_token TEXT NOT NULL DEFAULT '';

ALTER TABLE contacts ADD COLUMN direct_url TEXT NOT NULL DEFAULT '';
ALTER TABLE contacts ADD COLUMN direct_token TEXT NOT NULL DEFAULT '';
