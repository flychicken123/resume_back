-- Rollback: Clear Stripe price IDs
UPDATE subscription_plans
SET stripe_price_id = NULL, updated_at = CURRENT_TIMESTAMP
WHERE name IN ('premium', 'ultimate');
