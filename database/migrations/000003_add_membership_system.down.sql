-- Down migration: 000003_add_membership_system

DROP FUNCTION IF EXISTS increment_resume_usage(INTEGER);
DROP FUNCTION IF EXISTS check_resume_limit(INTEGER);

DROP INDEX IF EXISTS idx_users_is_admin;
DROP INDEX IF EXISTS idx_payment_history_user_id;
DROP INDEX IF EXISTS idx_resume_usage_period;
DROP INDEX IF EXISTS idx_resume_usage_user_id;
DROP INDEX IF EXISTS idx_user_subscriptions_status;
DROP INDEX IF EXISTS idx_user_subscriptions_user_id;

ALTER TABLE users DROP COLUMN IF EXISTS is_admin;
ALTER TABLE users DROP COLUMN IF EXISTS stripe_customer_id;
ALTER TABLE users DROP COLUMN IF EXISTS trial_end_date;
ALTER TABLE users DROP COLUMN IF EXISTS subscription_status;
ALTER TABLE users DROP COLUMN IF EXISTS subscription_plan_id;

DROP TABLE IF EXISTS payment_history;
DROP TABLE IF EXISTS resume_usage;
DROP TABLE IF EXISTS user_subscriptions;
DROP TABLE IF EXISTS subscription_plans;
