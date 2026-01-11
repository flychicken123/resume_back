-- Revert resume quota limits to original values
-- Free: 50 → 1 (weekly)
-- Premium: 1,500 → 30 (monthly)
-- Ultimate: 15,000 → 300 (monthly)

UPDATE subscription_plans
SET
    resume_limit = 1,
    features = '["1 resume per week", "Basic resume templates", "PDF export", "Email support"]'::jsonb
WHERE name = 'free';

UPDATE subscription_plans
SET
    resume_limit = 30,
    features = '["30 resumes per month", "All premium templates", "AI-powered optimization", "AI cover letters", "Priority support"]'::jsonb
WHERE name = 'premium';

UPDATE subscription_plans
SET
    resume_limit = 300,
    features = '["300 resumes per month", "All templates + exclusive designs", "Advanced AI features", "API access", "24/7 dedicated support"]'::jsonb
WHERE name = 'ultimate';
