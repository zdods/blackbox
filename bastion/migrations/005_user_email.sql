-- Account profile: an optional email address. Its only current use is the
-- Gravatar avatar shown on the account settings screen, but it's the natural
-- home for future profile fields too. Nullable; an empty/absent email simply
-- means no avatar is fetched and nothing is sent to Gravatar.
ALTER TABLE users ADD COLUMN IF NOT EXISTS email TEXT;
