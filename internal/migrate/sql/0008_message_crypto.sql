ALTER TABLE self_messaging_profiles ADD COLUMN encryption_public_key TEXT NOT NULL DEFAULT '';
ALTER TABLE self_messaging_profiles ADD COLUMN encryption_private_key_ref TEXT NOT NULL DEFAULT '';

ALTER TABLE contacts ADD COLUMN encryption_public_key TEXT NOT NULL DEFAULT '';
