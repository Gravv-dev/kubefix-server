-- Manual answer used when a challenge has no automated verification configured.

ALTER TABLE challenges
ADD COLUMN IF NOT EXISTS expected_answer TEXT NOT NULL DEFAULT '';
