package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"html"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"resumeai/models"
)

// GenerateUnsubscribeToken creates a signed token for a given email address
func GenerateUnsubscribeToken(email string) string {
	secret := getUnsubscribeSecret()
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(strings.ToLower(strings.TrimSpace(email))))
	return hex.EncodeToString(h.Sum(nil))
}

// ValidateUnsubscribeToken verifies the token matches the email
func ValidateUnsubscribeToken(email, token string) bool {
	expected := GenerateUnsubscribeToken(email)
	return hmac.Equal([]byte(expected), []byte(token))
}

func getUnsubscribeSecret() string {
	secret := strings.TrimSpace(os.Getenv("UNSUBSCRIBE_SECRET"))
	if secret == "" {
		// Fallback to JWT secret if UNSUBSCRIBE_SECRET not set
		secret = strings.TrimSpace(os.Getenv("JWT_SECRET"))
	}
	if secret == "" {
		secret = "default-unsubscribe-secret-change-me"
	}
	return secret
}

// HandleUnsubscribeGET handles GET requests for unsubscribe - returns HTML page
// This is used when users click the unsubscribe link in emails
func HandleUnsubscribeGET(userModel *models.UserModel) gin.HandlerFunc {
	return func(c *gin.Context) {
		email := strings.ToLower(strings.TrimSpace(c.Query("email")))
		token := strings.TrimSpace(c.Query("token"))

		if email == "" || token == "" {
			c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(unsubscribeErrorHTML("Invalid unsubscribe link. Please check the link from your email.")))
			return
		}

		// Validate the token
		if !ValidateUnsubscribeToken(email, token) {
			c.Data(http.StatusForbidden, "text/html; charset=utf-8", []byte(unsubscribeErrorHTML("Invalid or expired unsubscribe link.")))
			return
		}

		// Unsubscribe the user
		if err := userModel.SetEmailUnsubscribed(email, true); err != nil {
			c.Data(http.StatusInternalServerError, "text/html; charset=utf-8", []byte(unsubscribeErrorHTML("Failed to process your request. Please try again later.")))
			return
		}

		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(unsubscribeSuccessHTML(email)))
	}
}

func unsubscribeSuccessHTML(email string) string {
	safeEmail := html.EscapeString(email)
	return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Unsubscribed | HiHired</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            background: linear-gradient(135deg, #f5f7fa 0%, #e4e8ec 100%);
            padding: 20px;
        }
        .card {
            background: #fff;
            border-radius: 16px;
            box-shadow: 0 10px 40px rgba(0,0,0,0.1);
            padding: 40px;
            max-width: 480px;
            width: 100%;
            text-align: center;
        }
        .icon { font-size: 48px; color: #10b981; margin-bottom: 16px; }
        h1 { font-size: 24px; color: #1f2937; margin-bottom: 16px; }
        .email { font-size: 16px; color: #0b66c3; font-weight: 600; margin-bottom: 16px; word-break: break-all; }
        p { font-size: 14px; color: #6b7280; line-height: 1.6; margin-bottom: 24px; }
        .btn {
            display: inline-block;
            background: #0b66c3;
            color: #fff;
            text-decoration: none;
            padding: 12px 32px;
            border-radius: 8px;
            font-weight: 600;
        }
    </style>
</head>
<body>
    <div class="card">
        <div class="icon">&#10003;</div>
        <h1>You've Been Unsubscribed</h1>
        <div class="email">` + safeEmail + `</div>
        <p>You will no longer receive job match notification emails from HiHired. You can still use HiHired to build resumes anytime.</p>
        <a href="https://hihired.org" class="btn">Go to HiHired</a>
    </div>
</body>
</html>`
}

func unsubscribeErrorHTML(message string) string {
	safeMessage := html.EscapeString(message)
	return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Error | HiHired</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            background: linear-gradient(135deg, #f5f7fa 0%, #e4e8ec 100%);
            padding: 20px;
        }
        .card {
            background: #fff;
            border-radius: 16px;
            box-shadow: 0 10px 40px rgba(0,0,0,0.1);
            padding: 40px;
            max-width: 480px;
            width: 100%;
            text-align: center;
        }
        .icon { font-size: 48px; color: #ef4444; margin-bottom: 16px; }
        h1 { font-size: 24px; color: #1f2937; margin-bottom: 16px; }
        p { font-size: 14px; color: #6b7280; line-height: 1.6; margin-bottom: 24px; }
        .btn {
            display: inline-block;
            background: #0b66c3;
            color: #fff;
            text-decoration: none;
            padding: 12px 32px;
            border-radius: 8px;
            font-weight: 600;
        }
    </style>
</head>
<body>
    <div class="card">
        <div class="icon">&#10007;</div>
        <h1>Something Went Wrong</h1>
        <p>` + safeMessage + `</p>
        <a href="https://hihired.org" class="btn">Go to HiHired</a>
    </div>
</body>
</html>`
}
