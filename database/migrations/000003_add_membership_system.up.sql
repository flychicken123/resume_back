-- Migration: Add Membership System
-- Description: Adds tables for subscription plans, user subscriptions, and usage tracking

-- Create subscription plans table
CREATE TABLE IF NOT EXISTS subscription_plans (
    id SERIAL PRIMARY KEY,
    name VARCHAR(50) NOT NULL UNIQUE,
    display_name VARCHAR(100) NOT NULL,
    price DECIMAL(10, 2) NOT NULL,
    currency VARCHAR(3) DEFAULT 'USD',
    billing_period VARCHAR(20) DEFAULT 'monthly',
    resume_limit INTEGER NOT NULL,
    resume_period VARCHAR(20) NOT NULL, -- 'weekly', 'monthly', 'daily'
    features JSONB,
    stripe_price_id VARCHAR(255),
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create user subscriptions table
CREATE TABLE IF NOT EXISTS user_subscriptions (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    plan_id INTEGER NOT NULL REFERENCES subscription_plans(id),
    stripe_subscription_id VARCHAR(255),
    stripe_customer_id VARCHAR(255),
    status VARCHAR(50) NOT NULL DEFAULT 'active', -- active, canceled, past_due, trialing
    current_period_start TIMESTAMP NOT NULL,
    current_period_end TIMESTAMP NOT NULL,
    cancel_at_period_end BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id)
);

-- Create usage tracking table
CREATE TABLE IF NOT EXISTS resume_usage (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    resume_count INTEGER DEFAULT 0,
    period_start TIMESTAMP NOT NULL,
    period_end TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, period_start, period_end)
);

-- Create payment history table
CREATE TABLE IF NOT EXISTS payment_history (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    subscription_id INTEGER REFERENCES user_subscriptions(id),
    stripe_payment_intent_id VARCHAR(255),
    amount DECIMAL(10, 2) NOT NULL,
    currency VARCHAR(3) DEFAULT 'USD',
    status VARCHAR(50) NOT NULL,
    description TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Insert default subscription plans
INSERT INTO subscription_plans (name, display_name, price, resume_limit, resume_period, features) VALUES
('free', 'Free Plan', 0.00, 1, 'weekly',
 '{"features": ["1 resume per week", "Basic templates", "PDF export", "Email support"]}'),
('premium', 'Premium Plan', 7.99, 30, 'monthly',
 '{"features": [
   "30 resumes per month",
   "AI-generated cover letters included"
 ]}'),
('ultimate', 'Ultimate Plan', 29.99, 300, 'monthly',
 '{"features": [
   "300 resumes per month",
   "24 hours online support"
 ]}')
ON CONFLICT (name) DO NOTHING;

-- Add subscription fields to users table
ALTER TABLE users
ADD COLUMN IF NOT EXISTS subscription_plan_id INTEGER REFERENCES subscription_plans(id) DEFAULT 1,
ADD COLUMN IF NOT EXISTS subscription_status VARCHAR(50) DEFAULT 'free',
ADD COLUMN IF NOT EXISTS trial_end_date TIMESTAMP,
ADD COLUMN IF NOT EXISTS stripe_customer_id VARCHAR(255);

-- Create indexes for better performance
CREATE INDEX IF NOT EXISTS idx_user_subscriptions_user_id ON user_subscriptions(user_id);
CREATE INDEX IF NOT EXISTS idx_user_subscriptions_status ON user_subscriptions(status);
CREATE INDEX IF NOT EXISTS idx_resume_usage_user_id ON resume_usage(user_id);
CREATE INDEX IF NOT EXISTS idx_resume_usage_period ON resume_usage(period_start, period_end);
CREATE INDEX IF NOT EXISTS idx_payment_history_user_id ON payment_history(user_id);

-- Create function to check resume usage limit
CREATE OR REPLACE FUNCTION check_resume_limit(p_user_id INTEGER)
RETURNS TABLE(can_generate BOOLEAN, remaining_count INTEGER, reset_date TIMESTAMP) AS $$
DECLARE
    v_plan_id INTEGER;
    v_resume_limit INTEGER;
    v_resume_period VARCHAR(20);
    v_current_usage INTEGER;
    v_period_start TIMESTAMP;
    v_period_end TIMESTAMP;
BEGIN
    -- Get user's subscription plan
    SELECT COALESCE(u.subscription_plan_id, 1), sp.resume_limit, sp.resume_period
    INTO v_plan_id, v_resume_limit, v_resume_period
    FROM users u
    LEFT JOIN subscription_plans sp ON sp.id = COALESCE(u.subscription_plan_id, 1)
    WHERE u.id = p_user_id;

    -- Calculate period based on plan
    IF v_resume_period = 'weekly' THEN
        v_period_start := date_trunc('week', CURRENT_TIMESTAMP);
        v_period_end := v_period_start + INTERVAL '1 week';
    ELSIF v_resume_period = 'monthly' THEN
        v_period_start := date_trunc('month', CURRENT_TIMESTAMP);
        v_period_end := v_period_start + INTERVAL '1 month';
    ELSE -- daily
        v_period_start := date_trunc('day', CURRENT_TIMESTAMP);
        v_period_end := v_period_start + INTERVAL '1 day';
    END IF;

    -- Get current usage
    SELECT COALESCE(resume_count, 0)
    INTO v_current_usage
    FROM resume_usage
    WHERE user_id = p_user_id
    AND period_start = v_period_start
    AND period_end = v_period_end;

    -- Return result
    RETURN QUERY SELECT
        (v_current_usage < v_resume_limit) AS can_generate,
        (v_resume_limit - v_current_usage) AS remaining_count,
        v_period_end AS reset_date;
END;
$$ LANGUAGE plpgsql;

-- Create function to increment resume usage
CREATE OR REPLACE FUNCTION increment_resume_usage(p_user_id INTEGER)
RETURNS BOOLEAN AS $$
DECLARE
    v_resume_period VARCHAR(20);
    v_period_start TIMESTAMP;
    v_period_end TIMESTAMP;
BEGIN
    -- Get user's subscription plan period
    SELECT sp.resume_period
    INTO v_resume_period
    FROM users u
    LEFT JOIN subscription_plans sp ON sp.id = COALESCE(u.subscription_plan_id, 1)
    WHERE u.id = p_user_id;

    -- Calculate period
    IF v_resume_period = 'weekly' THEN
        v_period_start := date_trunc('week', CURRENT_TIMESTAMP);
        v_period_end := v_period_start + INTERVAL '1 week';
    ELSIF v_resume_period = 'monthly' THEN
        v_period_start := date_trunc('month', CURRENT_TIMESTAMP);
        v_period_end := v_period_start + INTERVAL '1 month';
    ELSE -- daily
        v_period_start := date_trunc('day', CURRENT_TIMESTAMP);
        v_period_end := v_period_start + INTERVAL '1 day';
    END IF;

    -- Increment or create usage record
    INSERT INTO resume_usage (user_id, resume_count, period_start, period_end)
    VALUES (p_user_id, 1, v_period_start, v_period_end)
    ON CONFLICT (user_id, period_start, period_end)
    DO UPDATE SET
        resume_count = resume_usage.resume_count + 1,
        updated_at = CURRENT_TIMESTAMP;

    RETURN TRUE;
END;
$$ LANGUAGE plpgsql;

-- Ensure admin flag exists on usersa
ALTER TABLE users
ADD COLUMN IF NOT EXISTS is_admin BOOLEAN DEFAULT false;

CREATE INDEX IF NOT EXISTS idx_users_is_admin ON users(is_admin);

-- Normalize plan pricing for testing environments
UPDATE subscription_plans
SET display_name = 'Premium Plan',
    price = 7.99,
    features = '{"features": [
      "30 resumes per month",
      "AI-generated cover letters included" 
    ]}',
    resume_limit = 30,
    resume_period = 'monthly',
    updated_at = CURRENT_TIMESTAMP
WHERE name = 'premium';

UPDATE subscription_plans
SET price = 29.99,
    features = '{"features": [
      "300 resumes per month",
      "24 hours online support"
    ]}',
    resume_limit = 300,
    resume_period = 'monthly',
    updated_at = CURRENT_TIMESTAMP
WHERE name = 'ultimate';

