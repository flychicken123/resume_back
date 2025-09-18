package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/stripe/stripe-go/v74"
	"github.com/stripe/stripe-go/v74/price"
	"github.com/stripe/stripe-go/v74/product"
)

func main() {
	// Set your Stripe API key from environment (matches backend configuration)
	apiKey := os.Getenv("STRIPE_SECRET_KEY")
	if apiKey == "" {
		log.Fatal("STRIPE_SECRET_KEY environment variable is not set")
	}
	stripe.Key = apiKey

	// Create or get products and prices
	fmt.Println("Setting up Stripe products...")

	// Create Premium product
	premiumProduct, err := product.New(&stripe.ProductParams{
		Name:        stripe.String("Premium Plan - Resume Builder"),
		Description: stripe.String("30 resumes per month"),
	})
	if err != nil {
		log.Printf("Error creating premium product: %v", err)
	} else {
		fmt.Printf("Premium Product ID: %s\n", premiumProduct.ID)
	}

	// Create Premium price
	premiumPrice, err := price.New(&stripe.PriceParams{
		Product:    stripe.String(premiumProduct.ID),
		UnitAmount: stripe.Int64(1),   // $0.01 in cents
		Currency:   stripe.String("usd"),
		Recurring: &stripe.PriceRecurringParams{
			Interval: stripe.String("month"),
		},
	})
	if err != nil {
		log.Printf("Error creating premium price: %v", err)
	} else {
		fmt.Printf("Premium Price ID: %s\n", premiumPrice.ID)
	}

	// Create Ultimate product
	ultimateProduct, err := product.New(&stripe.ProductParams{
		Name:        stripe.String("Ultimate Plan - Resume Builder"),
		Description: stripe.String("200 resumes per month"),
	})
	if err != nil {
		log.Printf("Error creating ultimate product: %v", err)
	} else {
		fmt.Printf("Ultimate Product ID: %s\n", ultimateProduct.ID)
	}

	// Create Ultimate price
	ultimatePrice, err := price.New(&stripe.PriceParams{
		Product:    stripe.String(ultimateProduct.ID),
		UnitAmount: stripe.Int64(2),   // $0.02 in cents
		Currency:   stripe.String("usd"),
		Recurring: &stripe.PriceRecurringParams{
			Interval: stripe.String("month"),
		},
	})
	if err != nil {
		log.Printf("Error creating ultimate price: %v", err)
	} else {
		fmt.Printf("Ultimate Price ID: %s\n", ultimatePrice.ID)
	}

	// Output results
	fmt.Println("\n=== Stripe Products Created Successfully ===")
	fmt.Printf("Premium Plan Price ID: %s ($0.01/month)\n", premiumPrice.ID)
	fmt.Printf("Ultimate Plan Price ID: %s ($0.02/month)\n", ultimatePrice.ID)
	fmt.Println("\nUpdate your database with these Price IDs:")
	fmt.Printf("UPDATE subscription_plans SET stripe_price_id = '%s' WHERE name = 'premium';\n", premiumPrice.ID)
	fmt.Printf("UPDATE subscription_plans SET stripe_price_id = '%s' WHERE name = 'ultimate';\n", ultimatePrice.ID)

	// Start a simple test server
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		html := fmt.Sprintf(`
		<!DOCTYPE html>
		<html>
		<head>
			<title>Stripe Test</title>
		</head>
		<body>
			<h1>Test Stripe Checkout</h1>
			<p>Premium Price ID: %s</p>
			<p>Ultimate Price ID: %s</p>
			<form action="http://localhost:8081/api/subscription/checkout" method="POST">
				<input type="hidden" name="plan_name" value="premium">
				<button type="submit">Subscribe to Premium ($0.01/month)</button>
			</form>
			<br>
			<form action="http://localhost:8081/api/subscription/checkout" method="POST">
				<input type="hidden" name="plan_name" value="ultimate">
				<button type="submit">Subscribe to Ultimate ($0.02/month)</button>
			</form>
		</body>
		</html>
		`, premiumPrice.ID, ultimatePrice.ID)

		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	})

	fmt.Println("\nStarting test server on http://localhost:4242")
	fmt.Println("Visit this URL to test Stripe checkout")

	// Save price IDs to environment for your app
	os.Setenv("PREMIUM_PRICE_ID", premiumPrice.ID)
	os.Setenv("ULTIMATE_PRICE_ID", ultimatePrice.ID)

	log.Fatal(http.ListenAndServe(":4242", nil))
}