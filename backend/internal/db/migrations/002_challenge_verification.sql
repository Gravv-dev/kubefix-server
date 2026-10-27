-- 002_challenge_verification.sql
-- Verification configuration for automated solution checking.

ALTER TABLE challenges
ADD COLUMN IF NOT EXISTS verification JSONB NOT NULL DEFAULT '{}';

-- Example verification payload (stored as JSONB):
-- {
--   "type": "command",
--   "container_cmd": ["sh", "-c", "cat /app/status.txt"],
--   "expected": "fixed",
--   "match": "contains"   // contains | equals | exit_zero
-- }

COMMENT ON COLUMN challenges.verification IS 'Automated solution verification configuration.';
