-- Migration: Set production Stripe price IDs
-- Prevents CreateOrUpdateStripeProducts() from creating duplicate products on each restart

UPDATE subscription_plans
SET stripe_price_id = 'price_1TCwLaPhHtaE1JFGNSz4wtnE',
    updated_at = CURRENT_TIMESTAMP
WHERE name = 'premium' AND (stripe_price_id IS NULL OR stripe_price_id = '');

UPDATE subscription_plans
SET stripe_price_id = 'price_1TCwLZPhHtaE1JFG1mKT5NWQ',
    updated_at = CURRENT_TIMESTAMP
WHERE name = 'ultimate' AND (stripe_price_id IS NULL OR stripe_price_id = '');
