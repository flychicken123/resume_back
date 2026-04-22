package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2/google"
)

// GCPQuotaHandler uses the project's service-account credentials to query
// Google's own Billing + Service Usage APIs and reports back what Google
// says about the project's state. Answers "are we on free tier" and
// "what are the actual Gemini quotas" from inside the app.
//
// Returns different error categories so the operator knows what kind of
// fix is needed:
//   - auth fail -> service account credentials broken
//   - 403 on billing -> SA lacks billing.resourceAssociations.list role
//   - billing shows no linked account -> BILLING IS THE PROBLEM
//   - billing shows linked account + 403 on quota -> SA lacks quota role
//   - quota data returned -> we can see the actual limits
func GCPQuotaHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		projectID := strings.TrimSpace(os.Getenv("GCP_PROJECT_ID"))
		if projectID == "" {
			c.JSON(200, gin.H{"ok": false, "stage": "env", "error": "GCP_PROJECT_ID not set"})
			return
		}

		credsJSON := buildSACredentialsJSON()
		if credsJSON == "" {
			c.JSON(200, gin.H{"ok": false, "stage": "env", "error": "GCP_* SA env vars incomplete"})
			return
		}

		creds, err := google.CredentialsFromJSON(ctx, []byte(credsJSON),
			"https://www.googleapis.com/auth/cloud-platform.read-only",
			"https://www.googleapis.com/auth/cloud-billing.readonly",
		)
		if err != nil {
			c.JSON(200, gin.H{"ok": false, "stage": "creds", "error": err.Error()})
			return
		}

		tok, err := creds.TokenSource.Token()
		if err != nil {
			c.JSON(200, gin.H{"ok": false, "stage": "token", "error": err.Error()})
			return
		}

		httpClient := &http.Client{Timeout: 8 * time.Second}

		result := gin.H{
			"ok":                 true,
			"project_id":         projectID,
			"sa_email":           strings.TrimSpace(os.Getenv("GCP_CLIENT_EMAIL")),
			"token_type":         tok.TokenType,
			"token_expires_in_s": int(time.Until(tok.Expiry).Seconds()),
		}

		// --- Check 1: billing linked? ---
		billingURL := fmt.Sprintf("https://cloudbilling.googleapis.com/v1/projects/%s/billingInfo", projectID)
		billingResp, billingBody, billingStatus, billingErr := doAuthedGET(ctx, httpClient, billingURL, tok.AccessToken)
		result["billing_http_status"] = billingStatus
		if billingErr != nil {
			result["billing_error"] = billingErr.Error()
		} else {
			result["billing_response"] = billingBody
			if billingResp != nil {
				if enabled, ok := billingResp["billingEnabled"].(bool); ok {
					result["billing_enabled"] = enabled
				}
				if acct, ok := billingResp["billingAccountName"].(string); ok && acct != "" {
					result["billing_account"] = acct
				} else if billingStatus == 200 {
					result["billing_account"] = "(none - project has no billing account linked)"
				}
			}
		}

		// --- Check 2: Service Usage API – list Gemini quota metrics ---
		quotaURL := fmt.Sprintf(
			"https://serviceusage.googleapis.com/v1/projects/%s/services/generativelanguage.googleapis.com/consumerQuotaMetrics?pageSize=50",
			projectID,
		)
		_, quotaBody, quotaStatus, quotaErr := doAuthedGET(ctx, httpClient, quotaURL, tok.AccessToken)
		result["quota_http_status"] = quotaStatus
		if quotaErr != nil {
			result["quota_error"] = quotaErr.Error()
		} else {
			// Only include quota body on success — the failure responses
			// are usually a single-line permission error we already capture via status.
			if quotaStatus == 200 {
				result["quota_response"] = quotaBody
			} else {
				result["quota_response_snippet"] = truncateJSON(quotaBody, 2000)
			}
		}

		// --- Check 3: Is the Generative Language API enabled? ---
		svcURL := fmt.Sprintf(
			"https://serviceusage.googleapis.com/v1/projects/%s/services/generativelanguage.googleapis.com",
			projectID,
		)
		svcResp, svcBody, svcStatus, svcErr := doAuthedGET(ctx, httpClient, svcURL, tok.AccessToken)
		result["service_http_status"] = svcStatus
		if svcErr != nil {
			result["service_error"] = svcErr.Error()
		} else if svcResp != nil {
			if state, ok := svcResp["state"].(string); ok {
				result["service_state"] = state
			}
			if svcStatus != 200 {
				result["service_response_snippet"] = truncateJSON(svcBody, 1000)
			}
		}

		// --- Interpretation ---
		result["interpretation"] = interpretGCPStatus(result)
		c.JSON(200, result)
	}
}

func buildSACredentialsJSON() string {
	required := []string{"GCP_PROJECT_ID", "GCP_PRIVATE_KEY_ID", "GCP_PRIVATE_KEY", "GCP_CLIENT_EMAIL", "GCP_CLIENT_ID", "GCP_CLIENT_X509_CERT_URL"}
	for _, k := range required {
		if strings.TrimSpace(os.Getenv(k)) == "" {
			return ""
		}
	}
	pk := os.Getenv("GCP_PRIVATE_KEY")
	pk = strings.ReplaceAll(pk, "\\n", "\n")
	pk = strings.Trim(pk, "'\"")

	payload := map[string]string{
		"type":                        "service_account",
		"project_id":                  os.Getenv("GCP_PROJECT_ID"),
		"private_key_id":              os.Getenv("GCP_PRIVATE_KEY_ID"),
		"private_key":                 pk,
		"client_email":                os.Getenv("GCP_CLIENT_EMAIL"),
		"client_id":                   os.Getenv("GCP_CLIENT_ID"),
		"auth_uri":                    "https://accounts.google.com/o/oauth2/auth",
		"token_uri":                   "https://oauth2.googleapis.com/token",
		"auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
		"client_x509_cert_url":        os.Getenv("GCP_CLIENT_X509_CERT_URL"),
	}
	b, _ := json.Marshal(payload)
	return string(b)
}

func doAuthedGET(ctx context.Context, client *http.Client, url, accessToken string) (map[string]interface{}, string, int, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, "", 0, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", 0, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	var parsed map[string]interface{}
	_ = json.Unmarshal(body, &parsed)
	return parsed, bodyStr, resp.StatusCode, nil
}

func interpretGCPStatus(r gin.H) string {
	billingStatus, _ := r["billing_http_status"].(int)
	quotaStatus, _ := r["quota_http_status"].(int)
	serviceStatus, _ := r["service_http_status"].(int)
	billingEnabled, _ := r["billing_enabled"].(bool)
	billingAccount, _ := r["billing_account"].(string)

	switch {
	case billingStatus == 403:
		return "The service account lacks billing.resourceAssociations.list permission. Grant it 'Billing Account Viewer' role or ask partner to check billing manually."
	case billingStatus == 200 && !billingEnabled:
		return "CONFIRMED: the project has NO billing account linked. This IS free tier. Link a billing account at https://console.cloud.google.com/billing/linkedaccount?project=" + r["project_id"].(string) + " to fix permanently."
	case billingStatus == 200 && billingEnabled:
		if billingAccount != "" && billingAccount != "(none - project has no billing account linked)" {
			base := "Billing IS enabled and linked to " + billingAccount + ". So this is not a free-tier issue."
			if quotaStatus == 200 {
				return base + " Quota response returned -- inspect quota_response to see the actual limits."
			}
			if quotaStatus == 403 {
				return base + " But we can't read quotas (SA lacks cloudquotas.quotas.get). Ask partner to check quotas at https://console.cloud.google.com/apis/api/generativelanguage.googleapis.com/quotas?project=" + r["project_id"].(string)
			}
			return base + " Service Usage call returned " + fmt.Sprintf("%d", quotaStatus) + " -- check snippet for details."
		}
		return "Billing enabled but account name empty. Unusual state."
	case serviceStatus == 404:
		return "Generative Language API appears not enabled on this project. Enable it at https://console.cloud.google.com/apis/library/generativelanguage.googleapis.com?project=" + r["project_id"].(string)
	default:
		return "Unexpected state. Inspect raw fields."
	}
}

func truncateJSON(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
