package main

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"flag"
	"fmt"
	"html"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"resumeai/config"
	"resumeai/database"
	"resumeai/services"
	"resumeai/utils"
)

type recipient struct {
	ID    int
	Email string
	Name  string
}

func main() {
	// Load local defaults. Production DB values should be supplied explicitly by
	// the deploy environment or shell overrides; .env.production in this repo is
	// a template and may contain unresolved placeholders.
	loadEnvFiles(".env", ".env.local")

	var (
		send           = flag.Bool("send", false, "actually send emails; default is dry-run")
		testEmail      = flag.String("test-email", "", "send exactly one test email to this address")
		limit          = flag.Int("limit", 0, "max recipients to load/send; 0 means no limit")
		delayMS        = flag.Int("delay-ms", 1000, "delay between sends in milliseconds")
		audience       = flag.String("audience", "opted-in", "recipient audience: opted-in or not-unsubscribed")
		excludeUserIDs = flag.String("exclude-user-ids", "", "comma-separated user IDs to exclude")
		extensionURL   = flag.String("extension-url", envOrDefault("CHROME_EXTENSION_URL", "https://chromewebstore.google.com/detail/hihired-auto-fill/obhbnkbkffabchelgomgbjglhplemidc"), "Chrome extension install URL")
		appURL         = flag.String("app-url", envOrDefault("APP_URL", "https://hihired.org"), "HiHired app URL")
		videoURL       = flag.String("video-url", envOrDefault("CHROME_EXTENSION_VIDEO_URL", "https://www.youtube.com/watch?v=yD2BszTyWj0"), "Chrome extension demo video URL")
		subject        = flag.String("subject", "New: Auto-fill job applications with HiHired's Chrome extension", "email subject")
	)
	flag.Parse()

	if strings.TrimSpace(*extensionURL) == "" {
		log.Fatal("missing -extension-url")
	}

	db := mustConnectDB()
	defer db.Close()

	emailSvc := services.NewEmailService(utils.NewLogger())
	if !emailSvc.Enabled() {
		log.Fatal("email service not configured; check SMTP_HOST/SMTP_PORT/SMTP_USERNAME/SMTP_PASSWORD/EMAIL_FROM/SUPPORT_EMAIL_TO")
	}

	if strings.TrimSpace(*testEmail) != "" {
		to := strings.TrimSpace(*testEmail)
		body := renderAnnouncementHTML("there", to, *extensionURL, *appURL, *videoURL)
		if !*send {
			fmt.Printf("DRY RUN: would send test email to %s\nSubject: %s\n", to, *subject)
			return
		}
		if err := emailSvc.SendHTMLEmail(to, *subject, body); err != nil {
			log.Fatalf("failed to send test email to %s: %v", to, err)
		}
		fmt.Printf("sent test email to %s\n", to)
		return
	}

	excludeIDs := parseIDSet(*excludeUserIDs)
	recipients, err := loadRecipients(context.Background(), db, *limit, *audience, excludeIDs)
	if err != nil {
		log.Fatalf("failed to load recipients: %v", err)
	}

	fmt.Printf("matched recipients: %d (audience=%s, excluded_ids=%d)\n", len(recipients), *audience, len(excludeIDs))
	if len(recipients) > 0 {
		preview := len(recipients)
		if preview > 10 {
			preview = 10
		}
		for i := 0; i < preview; i++ {
			fmt.Printf("  - #%d %s <%s>\n", recipients[i].ID, recipients[i].Name, recipients[i].Email)
		}
		if len(recipients) > preview {
			fmt.Printf("  ... %d more\n", len(recipients)-preview)
		}
	}

	if !*send {
		fmt.Println("DRY RUN ONLY. Re-run with -send to actually send, or use -test-email first.")
		return
	}

	var sent, failed int
	delay := time.Duration(*delayMS) * time.Millisecond
	for i, r := range recipients {
		name := firstNameOrThere(r.Name)
		body := renderAnnouncementHTML(name, r.Email, *extensionURL, *appURL, *videoURL)
		if err := emailSvc.SendHTMLEmail(r.Email, *subject, body); err != nil {
			failed++
			log.Printf("send failed user_id=%d email=%s: %v", r.ID, r.Email, err)
		} else {
			sent++
			log.Printf("sent %d/%d user_id=%d email=%s", i+1, len(recipients), r.ID, r.Email)
		}
		if i < len(recipients)-1 && delay > 0 {
			time.Sleep(delay)
		}
	}

	fmt.Printf("done: sent=%d failed=%d total=%d\n", sent, failed, len(recipients))
	if failed > 0 {
		os.Exit(1)
	}
}

func mustConnectDB() *sql.DB {
	cfg := config.GetAppConfig()
	db, err := database.Connect(
		cfg.Database.Host,
		strconv.Itoa(cfg.Database.Port),
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.DBName,
		cfg.Database.SSLMode,
		cfg.TimeZone,
	)
	if err != nil {
		log.Fatalf("database connect failed: %v", err)
	}
	return db
}

func loadRecipients(ctx context.Context, db *sql.DB, limit int, audience string, excludeIDs map[int]bool) ([]recipient, error) {
	where := `COALESCE(email_unsubscribed, FALSE) = FALSE
		  AND email IS NOT NULL
		  AND TRIM(email) <> ''`
	switch strings.ToLower(strings.TrimSpace(audience)) {
	case "opted-in":
		where = "marketing_opt_in = TRUE AND " + where
	case "not-unsubscribed":
		// Keep all users who have not unsubscribed.
	default:
		return nil, fmt.Errorf("unknown audience %q", audience)
	}

	query := fmt.Sprintf(`
		SELECT id, email, COALESCE(name, '')
		FROM users
		WHERE %s
		ORDER BY created_at DESC
	`, where)
	args := []any{}
	if limit > 0 {
		query += " LIMIT $1"
		args = append(args, limit)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []recipient
	for rows.Next() {
		var r recipient
		if err := rows.Scan(&r.ID, &r.Email, &r.Name); err != nil {
			return nil, err
		}
		r.Email = strings.TrimSpace(r.Email)
		if r.Email == "" || excludeIDs[r.ID] {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func renderAnnouncementHTML(name, recipientEmail, extensionURL, appURL, videoURL string) string {
	brand := "HiHired"
	safeName := html.EscapeString(firstNameOrThere(name))
	safeExtensionURL := html.EscapeString(extensionURL)
	safeAppURL := html.EscapeString(strings.TrimRight(appURL, "/"))
	safeVideoURL := html.EscapeString(videoURL)
	unsubscribeURL := html.EscapeString(buildUnsubscribeURL(appURL, recipientEmail))

	return fmt.Sprintf(`<!doctype html>
<html>
<body style="margin:0; padding:0; background:#f5f7fb; font-family:Arial,Helvetica,sans-serif; color:#111827;">
  <div style="max-width:680px; margin:0 auto; padding:32px 16px;">
    <div style="background:#ffffff; border:1px solid #e5e7eb; border-radius:16px; overflow:hidden; box-shadow:0 12px 32px rgba(15,23,42,0.08);">
      <div style="background:#0b66c3; color:#ffffff; padding:20px 24px;">
        <div style="font-size:20px; font-weight:700;">%s</div>
        <div style="font-size:13px; opacity:0.9; margin-top:4px;">New Chrome extension</div>
      </div>
      <div style="padding:28px 24px;">
        <p style="margin:0 0 14px; font-size:15px; color:#4b5563;">Hi %s,</p>
        <h1 style="margin:0 0 14px; font-size:24px; line-height:1.25; color:#0f172a;">Apply faster with HiHired's Chrome extension</h1>
        <p style="margin:0 0 16px; font-size:15px; line-height:1.6; color:#374151;">
          We just added a new Chrome extension that helps fill out job applications using the resume profile you already built in HiHired.
        </p>
        <p style="margin:0 0 18px; font-size:15px; line-height:1.6; color:#374151;">
          It can fill contact info, work experience, education, skills, and common application questions across sites like Workday, Greenhouse, Lever, LinkedIn, and more — so you don't have to retype the same details every time.
        </p>
        <div style="background:#f8fafc; border:1px solid #e5e7eb; border-radius:12px; padding:16px; margin:18px 0;">
          <div style="font-weight:700; margin-bottom:8px; color:#0f172a;">How it works</div>
          <ol style="margin:0; padding-left:20px; color:#374151; font-size:14px; line-height:1.7;">
            <li>Install the HiHired Chrome extension.</li>
            <li>Sign in with your HiHired account.</li>
            <li>Open a job application page.</li>
            <li>Click autofill, then review before submitting.</li>
          </ol>
        </div>
        <div style="margin:24px 0 10px;">
          <a href="%s" style="display:inline-block; background:#0b66c3; color:#ffffff; text-decoration:none; padding:13px 18px; border-radius:10px; font-weight:700;">Install the Chrome extension</a>
        </div>
        <div style="margin:18px 0; padding:16px; border:1px solid #dbeafe; border-radius:12px; background:#eff6ff;">
          <div style="font-weight:700; color:#0f172a; margin-bottom:6px;">See it in action</div>
          <p style="margin:0 0 10px; font-size:14px; line-height:1.6; color:#374151;">Watch the short demo: HiHired generates a resume from a real job description, opens the Chrome extension, and fills the application form without submitting it.</p>
          <a href="%s" style="color:#0b66c3; font-weight:700; text-decoration:none;">Watch the demo video →</a>
        </div>
        <p style="margin:18px 0 0; font-size:14px; line-height:1.6; color:#6b7280;">
          If your resume profile isn't finished yet, you can update it anytime at <a href="%s" style="color:#0b66c3; text-decoration:none;">hihired.org</a>.
        </p>
        <p style="margin:18px 0 0; font-size:15px; color:#374151;">Best,<br>The HiHired Team</p>
      </div>
      <div style="padding:18px 24px; border-top:1px solid #e5e7eb; font-size:12px; color:#6b7280;">
        <div>You received this because you signed up for HiHired updates.</div>
        <div style="margin-top:12px;">
          <a href="%s" style="display:inline-block; border:1px solid #d1d5db; color:#374151; background:#ffffff; text-decoration:none; padding:9px 14px; border-radius:8px; font-weight:700;">Unsubscribe from emails</a>
        </div>
      </div>
    </div>
  </div>
</body>
</html>`, brand, safeName, safeExtensionURL, safeVideoURL, safeAppURL, unsubscribeURL)
}

func parseIDSet(raw string) map[int]bool {
	out := make(map[int]bool)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.Atoi(part)
		if err == nil && id > 0 {
			out[id] = true
		}
	}
	return out
}

func firstNameOrThere(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "there"
	}
	fields := strings.Fields(name)
	if len(fields) == 0 {
		return "there"
	}
	return fields[0]
}

func buildUnsubscribeURL(appURL, email string) string {
	base := strings.TrimRight(appURL, "/")
	token := generateUnsubscribeToken(email)
	return fmt.Sprintf("%s/api/email/unsubscribe?email=%s&token=%s", base, url.QueryEscape(email), url.QueryEscape(token))
}

func generateUnsubscribeToken(email string) string {
	secret := strings.TrimSpace(os.Getenv("UNSUBSCRIBE_SECRET"))
	if secret == "" {
		secret = strings.TrimSpace(os.Getenv("JWT_SECRET"))
	}
	if secret == "" {
		secret = "default-unsubscribe-secret-change-me"
	}
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(strings.ToLower(strings.TrimSpace(email))))
	return hex.EncodeToString(h.Sum(nil))
}

func loadEnvFiles(paths ...string) {
	for _, p := range paths {
		loadEnvFile(p)
	}
}

func loadEnvFile(path string) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" || os.Getenv(key) != "" {
			continue
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, "\"'")
		_ = os.Setenv(key, value)
	}
}

func envOrDefault(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
