-- Increase resume quota limits by 50x for all plans
-- Free: 1 → 50 (weekly)
-- Premium: 30 → 1,500 (monthly)
-- Ultimate: 300 → 15,000 (monthly)

UPDATE subscription_plans
SET
    resume_limit = 50,
    features = '["50 resumes per week", "Basic resume templates", "PDF export", "Email support"]'::jsonb
WHERE name = 'free';

UPDATE subscription_plans
SET
    resume_limit = 1500,
    features = '["1,500 resumes per month", "All premium templates", "AI-powered optimization", "AI cover letters", "Priority support"]'::jsonb
WHERE name = 'premium';

UPDATE subscription_plans
SET
    resume_limit = 15000,
    features = '["15,000 resumes per month", "All templates + exclusive designs", "Advanced AI features", "API access", "24/7 dedicated support"]'::jsonb
WHERE name = 'ultimate';
