-- Down migration: 000013_add_user_marketing_opt_in

ALTER TABLE users DROP COLUMN IF EXISTS signup_plan_preference;
ALTER TABLE users DROP COLUMN IF EXISTS marketing_opt_in_source;
ALTER TABLE users DROP COLUMN IF EXISTS marketing_opted_at;
ALTER TABLE users DROP COLUMN IF EXISTS marketing_opt_in;
