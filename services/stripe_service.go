package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/stripe/stripe-go/v74"
	"github.com/stripe/stripe-go/v74/checkout/session"
	"github.com/stripe/stripe-go/v74/customer"
	"github.com/stripe/stripe-go/v74/price"
	"github.com/stripe/stripe-go/v74/product"
	"github.com/stripe/stripe-go/v74/subscription"
	"github.com/stripe/stripe-go/v74/webhook"
)

type stripePlan struct {
	ID            int
	Name          string
	DisplayName   string
	PriceCents    int64
	StripePriceID string
}

type StripeService struct {
	db           *sql.DB
	planCache    map[string]stripePlan
	cacheMu      sync.RWMutex
	lastPlanSync time.Time
}

func NewStripeService(db *sql.DB) *StripeService {
	// Set Stripe API key from environment
	stripe.Key = os.Getenv("STRIPE_SECRET_KEY")
	if stripe.Key == "" {
		fmt.Println("Warning: STRIPE_SECRET_KEY not set")
	}
	return &StripeService{
		db:        db,
		planCache: make(map[string]stripePlan),
	}
}

func (s *StripeService) getStripePlans() ([]stripePlan, error) {
	s.cacheMu.RLock()
	if len(s.planCache) > 0 && time.Since(s.lastPlanSync) < 5*time.Minute {
		plans := make([]stripePlan, 0, len(s.planCache))
		for _, p := range s.planCache {
			plans = append(plans, p)
		}
		s.cacheMu.RUnlock()
		return plans, nil
	}
	s.cacheMu.RUnlock()

	rows, err := s.db.Query(`
        SELECT id, name, display_name, price, stripe_price_id
        FROM subscription_plans
        WHERE is_active = true
    `)
	if err != nil {
		return nil, fmt.Errorf("failed to load plans: %w", err)
	}
	defer rows.Close()

	var plans []stripePlan
	for rows.Next() {
		var (
			id            int
			name          string
			displayName   string
			price         float64
			stripePriceID sql.NullString
		)
		if err := rows.Scan(&id, &name, &displayName, &price, &stripePriceID); err != nil {
			return nil, fmt.Errorf("failed to scan plan: %w", err)
		}

		priceCents := int64(math.Round(price * 100))
		if priceCents <= 0 {
			continue
		}

		plans = append(plans, stripePlan{
			ID:            id,
			Name:          name,
			DisplayName:   displayName,
			PriceCents:    priceCents,
			StripePriceID: stripePriceID.String,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read plans: %w", err)
	}

	s.cacheMu.Lock()
	s.planCache = make(map[string]stripePlan, len(plans))
	for _, p := range plans {
		s.planCache[p.Name] = p
	}
	s.lastPlanSync = time.Now()
	s.cacheMu.Unlock()

	return plans, nil
}

func (s *StripeService) getPlanByName(planName string) (stripePlan, error) {
	if planName == "" {
		return stripePlan{}, fmt.Errorf("plan name is required")
	}

	if _, err := s.getStripePlans(); err != nil {
		return stripePlan{}, err
	}

	s.cacheMu.RLock()
	plan, ok := s.planCache[planName]
	s.cacheMu.RUnlock()
	if !ok {
		return stripePlan{}, fmt.Errorf("plan not found: %s", planName)
	}

	return plan, nil
}

func (s *StripeService) getPlanByPriceID(priceID string) (stripePlan, bool, error) {
	if priceID == "" {
		return stripePlan{}, false, nil
	}

	if _, err := s.getStripePlans(); err != nil {
		return stripePlan{}, false, err
	}

	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()

	for _, plan := range s.planCache {
		if plan.StripePriceID == priceID {
			return plan, true, nil
		}
	}

	return stripePlan{}, false, nil
}

func (s *StripeService) CreateOrUpdateStripeProducts() error {
	plans, err := s.getStripePlans()
	if err != nil {
		return err
	}
	if len(plans) == 0 {
		return fmt.Errorf("no active subscription plans to sync with Stripe")
	}

	for _, plan := range plans {
		productParams := &stripe.ProductParams{
			Name:        stripe.String(plan.DisplayName),
			Description: stripe.String(fmt.Sprintf("%s - Resume Builder", plan.DisplayName)),
		}
		productParams.AddMetadata("plan_name", plan.Name)

		prod, err := product.New(productParams)
		if err != nil {
			return fmt.Errorf("failed to create product %s: %v", plan.Name, err)
		}

		priceParams := &stripe.PriceParams{
			Product:    stripe.String(prod.ID),
			UnitAmount: stripe.Int64(plan.PriceCents),
			Currency:   stripe.String("usd"),
			Recurring: &stripe.PriceRecurringParams{
				Interval: stripe.String("month"),
			},
		}
		priceParams.AddMetadata("plan_name", plan.Name)

		priceObj, err := price.New(priceParams)
		if err != nil {
			return fmt.Errorf("failed to create price for %s: %v", plan.Name, err)
		}

		if _, err := s.db.Exec(`
			UPDATE subscription_plans
			SET stripe_price_id = $1, updated_at = CURRENT_TIMESTAMP
			WHERE id = $2
		`, priceObj.ID, plan.ID); err != nil {
			return fmt.Errorf("failed to update plan with stripe price ID: %v", err)
		}
		plan.StripePriceID = priceObj.ID
		s.cacheMu.Lock()
		s.planCache[plan.Name] = plan
		s.cacheMu.Unlock()
	}

	return nil
}
func (s *StripeService) CreateCheckoutSession(userID int, planName, successURL, cancelURL string) (*stripe.CheckoutSession, error) {
	// Get plan details
	plan, err := s.getPlanByName(planName)
	if err != nil {
		return nil, fmt.Errorf("plan not found: %v", err)
	}

	if plan.StripePriceID == "" {
		return nil, fmt.Errorf("stripe price ID not configured for plan %s", planName)
	}

	planID := plan.ID
	priceID := plan.StripePriceID

	// Get user email
	var email string
	err = s.db.QueryRow("SELECT email FROM users WHERE id = $1", userID).Scan(&email)
	if err != nil {
		return nil, fmt.Errorf("user not found: %v", err)
	}

	// Create checkout session
	params := &stripe.CheckoutSessionParams{
		Mode: stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(priceID),
				Quantity: stripe.Int64(1),
			},
		},
		SuccessURL:          stripe.String(successURL),
		CancelURL:           stripe.String(cancelURL),
		CustomerEmail:       stripe.String(email),
		AllowPromotionCodes: stripe.Bool(true),
		SubscriptionData: &stripe.CheckoutSessionSubscriptionDataParams{
			TrialPeriodDays: stripe.Int64(7), // 7-day free trial
		},
	}
	params.AddMetadata("user_id", fmt.Sprintf("%d", userID))
	params.AddMetadata("plan_id", fmt.Sprintf("%d", planID))

	return session.New(params)
}

// CreateCustomerPortalSession creates a session for customer to manage subscription
func (s *StripeService) CreateCustomerPortalSession(userID int, returnURL string) (string, error) {
	// Get customer ID
	var customerID string
	err := s.db.QueryRow(`
		SELECT stripe_customer_id
		FROM users
		WHERE id = $1
	`, userID).Scan(&customerID)
	if err != nil || customerID == "" {
		return "", fmt.Errorf("no stripe customer found for user")
	}

	// Create portal session using Stripe CLI or API
	// Note: You need to configure Customer Portal in Stripe Dashboard first
	portalURL := fmt.Sprintf("https://billing.stripe.com/p/session/YOUR_PORTAL_CONFIG?customer=%s", customerID)

	return portalURL, nil
}

// HandleWebhook processes Stripe webhook events
func (s *StripeService) HandleWebhook(payload []byte, signature string) error {
	webhookSecret := os.Getenv("STRIPE_WEBHOOK_SECRET")
	if webhookSecret == "" {
		return fmt.Errorf("STRIPE_WEBHOOK_SECRET not configured")
	}

	event, err := webhook.ConstructEvent(payload, signature, webhookSecret)
	if err != nil {
		return fmt.Errorf("webhook signature verification failed: %v", err)
	}

	switch event.Type {
	case "checkout.session.completed":
		var session stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
			return fmt.Errorf("error parsing checkout session: %v", err)
		}
		return s.handleCheckoutCompleted(&session)

	case "customer.subscription.updated":
		var sub stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
			return fmt.Errorf("error parsing subscription: %v", err)
		}
		return s.handleSubscriptionUpdated(&sub)

	case "customer.subscription.deleted":
		var sub stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
			return fmt.Errorf("error parsing subscription: %v", err)
		}
		return s.handleSubscriptionCanceled(&sub)

	case "invoice.payment_failed":
		// Handle failed payment
		fmt.Printf("Payment failed for event: %s\n", event.ID)
	}

	return nil
}

// handleCheckoutCompleted processes successful checkout
func (s *StripeService) handleCheckoutCompleted(session *stripe.CheckoutSession) error {
	userID := session.Metadata["user_id"]
	planID := session.Metadata["plan_id"]

	// Create customer if needed
	if session.Customer == nil {
		customerParams := &stripe.CustomerParams{
			Email: stripe.String(session.CustomerEmail),
		}
		customerParams.AddMetadata("user_id", userID)
		cust, err := customer.New(customerParams)
		if err != nil {
			return err
		}
		session.Customer = &stripe.Customer{ID: cust.ID}
	}

	// Update user with customer ID
	if _, err := s.db.Exec(`
		UPDATE users
		SET stripe_customer_id = $1,
		    subscription_plan_id = $2,
		    subscription_status = 'active'
		WHERE id = $3
	`, session.Customer.ID, planID, userID); err != nil {
		return err
	}

	// Create subscription record
	if _, err := s.db.Exec(`
		INSERT INTO user_subscriptions (
			user_id, plan_id, stripe_subscription_id, stripe_customer_id,
			status, current_period_start, current_period_end
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id) DO UPDATE SET
			plan_id = $2,
			stripe_subscription_id = $3,
			status = $5,
			current_period_start = $6,
			current_period_end = $7,
			updated_at = CURRENT_TIMESTAMP
	`, userID, planID, session.Subscription.ID, session.Customer.ID,
		"active", time.Now(), time.Now().AddDate(0, 1, 0)); err != nil {
		return err
	}

	return nil
}

// handleSubscriptionUpdated processes subscription updates
func (s *StripeService) handleSubscriptionUpdated(sub *stripe.Subscription) error {
	customerID := sub.Customer.ID

	// Update subscription status
	if _, err := s.db.Exec(`
		UPDATE user_subscriptions
		SET status = $1,
		    current_period_start = $2,
		    current_period_end = $3,
		    cancel_at_period_end = $4,
		    updated_at = CURRENT_TIMESTAMP
		WHERE stripe_customer_id = $5
	`, sub.Status,
		time.Unix(sub.CurrentPeriodStart, 0),
		time.Unix(sub.CurrentPeriodEnd, 0),
		sub.CancelAtPeriodEnd,
		customerID); err != nil {
		return err
	}

	// Update user status
	if _, err := s.db.Exec(`
		UPDATE users
		SET subscription_status = $1
		WHERE stripe_customer_id = $2
	`, sub.Status, customerID); err != nil {
		return err
	}

	return nil
}

// handleSubscriptionCanceled processes subscription cancellation
func (s *StripeService) handleSubscriptionCanceled(sub *stripe.Subscription) error {
	customerID := sub.Customer.ID

	// Update subscription to canceled
	if _, err := s.db.Exec(`
		UPDATE user_subscriptions
		SET status = 'canceled',
		    updated_at = CURRENT_TIMESTAMP
		WHERE stripe_customer_id = $1
	`, customerID); err != nil {
		return err
	}

	// Revert user to free plan
	if _, err := s.db.Exec(`
		UPDATE users
		SET subscription_plan_id = 1,
		    subscription_status = 'free'
		WHERE stripe_customer_id = $1
	`, customerID); err != nil {
		return err
	}

	return nil
}

// CheckUsageLimit checks if user can generate more resumes
func (s *StripeService) CheckUsageLimit(userID int) (canGenerate bool, remaining int, resetDate time.Time, err error) {
	row := s.db.QueryRow(`
		SELECT can_generate, remaining_count, reset_date
		FROM check_resume_limit($1)
	`, userID)

	err = row.Scan(&canGenerate, &remaining, &resetDate)
	return
}

// IncrementUsage increments the resume usage count
func (s *StripeService) IncrementUsage(userID int) error {
	var success bool
	err := s.db.QueryRow(`SELECT increment_resume_usage($1)`, userID).Scan(&success)
	if err != nil {
		return err
	}
	if !success {
		return fmt.Errorf("failed to increment usage")
	}
	return nil
}

// ConfirmCheckoutSession verifies a Checkout Session and persists the subscription locally.
func (s *StripeService) ConfirmCheckoutSession(userID int, sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("missing session_id")
	}
	// Retrieve the session from Stripe
	// Expand subscription to inspect items if metadata is missing
	sessParams := &stripe.CheckoutSessionParams{}
	sessParams.AddExpand("subscription")
	sessParams.AddExpand("line_items")
	sess, err := session.Get(sessionID, sessParams)
	if err != nil {
		return fmt.Errorf("failed to get session: %v", err)
	}
	if sess == nil {
		return fmt.Errorf("invalid session")
	}
	// Basic status validation
	if sess.Status != stripe.CheckoutSessionStatusComplete {
		if sess.PaymentStatus != stripe.CheckoutSessionPaymentStatusPaid {
			return fmt.Errorf("session not complete or paid yet")
		}
	}
	// Validate the session belongs to the authenticated user when possible
	if sess.Metadata != nil {
		if uid := sess.Metadata["user_id"]; uid != "" && uid != fmt.Sprintf("%d", userID) {
			return fmt.Errorf("session does not belong to user")
		}
	}
	// Determine plan from metadata
	var planID int
	if sess.Metadata != nil {
		if pid := sess.Metadata["plan_id"]; pid != "" {
			if p, convErr := strconv.Atoi(pid); convErr == nil {
				planID = p
			}
		}
	}
	if planID == 0 {
		// Try derive from subscription's price
		var subObj *stripe.Subscription
		if sess.Subscription != nil {
			// Expand items and prices
			subParams := &stripe.SubscriptionParams{}
			subParams.AddExpand("items.data.price")
			sObj, sErr := subscription.Get(sess.Subscription.ID, subParams)
			if sErr == nil {
				subObj = sObj
			}
		}
		if subObj != nil && len(subObj.Items.Data) > 0 {
			priceID := subObj.Items.Data[0].Price.ID
			if priceID != "" {
				if plan, found, cacheErr := s.getPlanByPriceID(priceID); cacheErr != nil {
					return cacheErr
				} else if found {
					planID = plan.ID
				} else {
					// Fallback to database lookup if the price is not in cache yet
					_ = s.db.QueryRow(`SELECT id FROM subscription_plans WHERE stripe_price_id = $1`, priceID).Scan(&planID)
				}
			}
		}
		if planID == 0 {
			return fmt.Errorf("unable to determine plan from session")
		}
	}
	// Ensure we have a customer ID
	if sess.Customer == nil {
		// Try to reuse existing customer
		var existing string
		_ = s.db.QueryRow("SELECT stripe_customer_id FROM users WHERE id = $1", userID).Scan(&existing)
		if existing != "" {
			sess.Customer = &stripe.Customer{ID: existing}
		} else {
			// Create a customer from email
			cparams := &stripe.CustomerParams{Email: stripe.String(sess.CustomerEmail)}
			cparams.AddMetadata("user_id", fmt.Sprintf("%d", userID))
			cust, cErr := customer.New(cparams)
			if cErr != nil {
				return fmt.Errorf("failed to create customer: %v", cErr)
			}
			sess.Customer = &stripe.Customer{ID: cust.ID}
		}
	}
	var subscriptionID string
	if sess.Subscription != nil {
		subscriptionID = sess.Subscription.ID
	}
	// Update user record
	if _, err := s.db.Exec(`
        UPDATE users
        SET stripe_customer_id = $1,
            subscription_plan_id = $2,
            subscription_status = 'active'
        WHERE id = $3
    `, sess.Customer.ID, planID, userID); err != nil {
		return err
	}
	// Upsert subscription record
	_, err = s.db.Exec(`
        INSERT INTO user_subscriptions (
            user_id, plan_id, stripe_subscription_id, stripe_customer_id,
            status, current_period_start, current_period_end
        ) VALUES ($1, $2, $3, $4, $5, $6, $7)
        ON CONFLICT (user_id) DO UPDATE SET
            plan_id = $2,
            stripe_subscription_id = $3,
            status = $5,
            current_period_start = $6,
            current_period_end = $7,
            updated_at = CURRENT_TIMESTAMP
    `, userID, planID, subscriptionID, sess.Customer.ID, "active", time.Now(), time.Now().AddDate(0, 1, 0))
	return err
}
