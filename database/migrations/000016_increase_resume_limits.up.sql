-- Increase resume quota limits by 100x for all plans
-- Free: 1 → 100 (weekly)
-- Premium: 30 → 3000 (monthly)
-- Ultimate: 300 → 30000 (monthly)

UPDATE subscription_plans
SET
    resume_limit = 100,
    features = '["100 resumes per week", "Basic resume templates", "PDF export", "Email support"]'::jsonb
WHERE name = 'free';

UPDATE subscription_plans
SET
    resume_limit = 3000,
    features = '["3,000 resumes per month", "All premium templates", "AI-powered optimization", "AI cover letters", "Priority support"]'::jsonb
WHERE name = 'premium';

UPDATE subscription_plans
SET
    resume_limit = 30000,
    features = '["30,000 resumes per month", "All templates + exclusive designs", "Advanced AI features", "API access", "24/7 dedicated support"]'::jsonb
WHERE name = 'ultimate';
