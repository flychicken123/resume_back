package services

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	stdhtml "html"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"resumeai/models"
	"resumeai/utils"
)

const defaultJobNotifyInterval = 72 * time.Hour

// JobMatchNotifier periodically computes matches for all users and emails them.
type JobMatchNotifier struct {
	resumes *models.ResumeModel
	matcher ResumeJobMatcher
	email   *EmailService
	logger  *utils.Logger

	runAllMu     sync.Mutex
	runAllStatus JobMatchRunAllStatus
}

type JobMatchManualRunResult struct {
	UserID         int    `json:"user_id"`
	RecipientEmail string `json:"recipient_email"`
	Matches        int    `json:"matches"`
	EmailSent      bool   `json:"email_sent"`
}

type JobMatchRunAllResult struct {
	RequestedLimit int                       `json:"requested_limit"`
	Processed      int                       `json:"processed"`
	EmailsSent     int                       `json:"emails_sent"`
	MatchesFound   int                       `json:"matches_found"`
	Errors         int                       `json:"errors"`
	Results        []JobMatchManualRunResult `json:"results,omitempty"`
}

type JobMatchRunAllStatus struct {
	Running    bool                  `json:"running"`
	StartedAt  *time.Time            `json:"started_at,omitempty"`
	FinishedAt *time.Time            `json:"finished_at,omitempty"`
	LastError  string                `json:"last_error,omitempty"`
	LastResult *JobMatchRunAllResult `json:"last_result,omitempty"`
}

// NewJobMatchNotifier wires the notifier dependencies.
func NewJobMatchNotifier(resumes *models.ResumeModel, matcher ResumeJobMatcher, email *EmailService, logger *utils.Logger) *JobMatchNotifier {
	if logger == nil {
		logger = utils.NewLogger()
	}
	return &JobMatchNotifier{
		resumes: resumes,
		matcher: matcher,
		email:   email,
		logger:  logger,
	}
}

// Start begins the periodic notifier loop.
func (n *JobMatchNotifier) Start(ctx context.Context, interval time.Duration) {
	if n == nil || n.resumes == nil || n.matcher == nil {
		return
	}
	if interval <= 0 {
		interval = defaultJobNotifyInterval
	}
	go n.loop(ctx, interval)
}

func (n *JobMatchNotifier) loop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	if strings.EqualFold(strings.TrimSpace(os.Getenv("JOB_MATCH_NOTIFIER_RUN_ON_STARTUP")), "true") {
		n.runOnce(ctx)
	} else {
		n.logger.Info("job match notifier startup run disabled", nil)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n.runOnce(ctx)
		}
	}
}

func (n *JobMatchNotifier) runOnce(ctx context.Context) {
	if n.email == nil || !n.email.Enabled() {
		n.logger.Info("job match notifier skipped (email disabled)", nil)
		return
	}

	resumes, err := n.resumes.ListWithUsers()
	if err != nil {
		n.logger.Warn("job match notifier: failed to list resumes", map[string]interface{}{"error": err.Error()})
		return
	}

	for _, item := range resumes {
		select {
		case <-ctx.Done():
			return
		default:
		}

		_, _, _, _ = n.processResume(ctx, item, "", true)
	}
}

func (n *JobMatchNotifier) processResume(ctx context.Context, item *models.ResumeWithUser, emailOverride string, sendEmail bool) (int, bool, string, error) {
	if item == nil {
		return 0, false, "", errors.New("missing resume")
	}

	resume := item.Resume
	recipient := strings.TrimSpace(item.UserEmail)
	if emailOverride != "" {
		recipient = strings.TrimSpace(emailOverride)
	}
	if recipient == "" {
		return 0, false, "", errors.New("missing recipient email")
	}

	skills := parseSkills(resume.Skills)
	summary := parseString(resume.Summary)

	experienceText := strings.TrimSpace(resume.Experience)
	if experienceText == "" {
		if exps, err := n.resumes.GetExperiencesByResumeID(resume.ID); err == nil {
			experienceText = summariseExperiences(exps)
		}
	}

	hashInput := ResumeHashInput{
		Position:       resume.Name,
		Name:           resume.Name,
		Email:          resume.Email,
		Summary:        summary,
		Experience:     experienceText,
		Education:      resume.Education,
		Location:       resume.Location,
		Skills:         skills,
		JobDescription: resume.JobDescription,
	}
	resumeHash := DeriveResumeHash(hashInput)

	matchInput := ResumeJobMatchInput{
		UserID:            resume.UserID,
		ResumeHash:        resumeHash,
		Position:          resume.Name,
		Summary:           summary,
		Experience:        experienceText,
		Education:         resume.Education,
		JobDescription:    resume.JobDescription,
		PreferredLocation: resume.Location,
		Skills:            skills,
		CandidateJobLimit: 1000,
		MaxResults:        10,
	}

	matches, err := n.matcher.MatchAndStore(ctx, matchInput)
	if err != nil {
		return 0, false, recipient, err
	}
	if len(matches) == 0 {
		return 0, false, recipient, nil
	}

	if !sendEmail {
		return len(matches), false, recipient, nil
	}

	// Skip sending email if user has unsubscribed or opted out of marketing (unless using email override for testing)
	if emailOverride == "" {
		if item.EmailUnsubscribed {
			n.logger.Debug("skipping email for unsubscribed user", map[string]interface{}{"user_id": resume.UserID, "email": recipient})
			return len(matches), false, recipient, nil
		}
		if !item.MarketingOptIn {
			n.logger.Debug("skipping email for user who opted out of marketing", map[string]interface{}{"user_id": resume.UserID, "email": recipient})
			return len(matches), false, recipient, nil
		}
	}

	body := formatMatchEmailBody(item.UserName, recipient, matches)
	if err := n.email.SendHTMLEmail(recipient, "New job matches for your resume", body); err != nil {
		return len(matches), false, recipient, err
	}
	return len(matches), true, recipient, nil
}

// RunOnceForUser computes matches for a single user and sends the email if there are any matches.
func (n *JobMatchNotifier) RunOnceForUser(ctx context.Context, userID int, emailOverride string) (*JobMatchManualRunResult, error) {
	if n == nil || n.resumes == nil || n.matcher == nil {
		return nil, errors.New("job match notifier not configured")
	}
	if n.email == nil || !n.email.Enabled() {
		return nil, errors.New("email service not configured")
	}
	if userID <= 0 {
		return nil, errors.New("invalid user id")
	}

	item, err := n.resumes.GetWithUserByUserID(userID)
	if err != nil {
		return nil, err
	}

	matches, sent, recipient, err := n.processResume(ctx, item, emailOverride, true)
	if err != nil {
		n.logger.Warn("job match notifier manual run failed", map[string]interface{}{"user_id": userID, "error": err.Error()})
		return nil, err
	}
	if sent {
		n.logger.Info("job match notifier manual run sent", map[string]interface{}{"user_id": userID, "matches": matches})
	} else {
		n.logger.Info("job match notifier manual run completed", map[string]interface{}{"user_id": userID, "matches": matches})
	}

	return &JobMatchManualRunResult{
		UserID:         userID,
		RecipientEmail: recipient,
		Matches:        matches,
		EmailSent:      sent,
	}, nil
}

// RunOnceForAll computes matches for all users and optionally sends emails.
// For safety, callers should gate this behind strong auth + explicit confirmation.
func (n *JobMatchNotifier) RunOnceForAll(ctx context.Context, limit int, emailOverride string, sendEmail bool) (*JobMatchRunAllResult, error) {
	if n == nil || n.resumes == nil || n.matcher == nil {
		return nil, errors.New("job match notifier not configured")
	}
	if sendEmail && (n.email == nil || !n.email.Enabled()) {
		return nil, errors.New("email service not configured")
	}

	resumes, err := n.resumes.ListWithUsers()
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(resumes) > limit {
		resumes = resumes[:limit]
	}

	result := &JobMatchRunAllResult{
		RequestedLimit: limit,
	}

	for _, item := range resumes {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}
		if item == nil {
			continue
		}
		userID := item.Resume.UserID
		matches, sent, _, err := n.processResume(ctx, item, emailOverride, sendEmail)
		result.Processed++
		result.MatchesFound += matches
		if sent {
			result.EmailsSent++
		}
		if err != nil {
			result.Errors++
			n.logger.Warn("job match notifier run-all failed", map[string]interface{}{"user_id": userID, "error": err.Error()})
		}
	}

	return result, nil
}

func (n *JobMatchNotifier) TriggerRunAll(limit int, emailOverride string, sendEmail bool) (JobMatchRunAllStatus, error) {
	if n == nil || n.resumes == nil || n.matcher == nil {
		return JobMatchRunAllStatus{}, errors.New("job match notifier not configured")
	}
	if sendEmail && (n.email == nil || !n.email.Enabled()) {
		return JobMatchRunAllStatus{}, errors.New("email service not configured")
	}

	n.runAllMu.Lock()
	defer n.runAllMu.Unlock()

	if n.runAllStatus.Running {
		return n.runAllStatus, errors.New("run-all already in progress")
	}

	started := time.Now()
	n.runAllStatus = JobMatchRunAllStatus{
		Running:    true,
		StartedAt:  &started,
		FinishedAt: nil,
		LastError:  "",
		LastResult: nil,
	}

	go func() {
		result, err := n.RunOnceForAll(context.Background(), limit, emailOverride, sendEmail)
		finished := time.Now()

		n.runAllMu.Lock()
		defer n.runAllMu.Unlock()

		n.runAllStatus.Running = false
		n.runAllStatus.FinishedAt = &finished
		if err != nil {
			n.runAllStatus.LastError = err.Error()
			n.runAllStatus.LastResult = nil
			return
		}
		n.runAllStatus.LastError = ""
		n.runAllStatus.LastResult = result
	}()

	return n.runAllStatus, nil
}

func (n *JobMatchNotifier) RunAllStatus() JobMatchRunAllStatus {
	if n == nil {
		return JobMatchRunAllStatus{}
	}
	n.runAllMu.Lock()
	defer n.runAllMu.Unlock()
	return n.runAllStatus
}

func parseString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	var any interface{}
	if err := json.Unmarshal(raw, &any); err == nil {
		return strings.TrimSpace(fmt.Sprint(any))
	}
	return strings.TrimSpace(string(raw))
}

func parseSkills(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var slice []string
	if err := json.Unmarshal(raw, &slice); err == nil {
		return normalizeSkills(slice)
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return normalizeSkills(strings.Split(s, ","))
	}
	return nil
}

func normalizeSkills(skills []string) []string {
	var normalized []string
	seen := make(map[string]struct{})
	for _, skill := range skills {
		s := strings.TrimSpace(skill)
		if s == "" {
			continue
		}
		lower := strings.ToLower(s)
		if _, ok := seen[lower]; ok {
			continue
		}
		seen[lower] = struct{}{}
		normalized = append(normalized, s)
	}
	return normalized
}

func summariseExperiences(exps []models.ExperienceRecord) string {
	if len(exps) == 0 {
		return ""
	}
	var parts []string
	for _, exp := range exps {
		var lineParts []string
		if exp.JobTitle != "" {
			lineParts = append(lineParts, exp.JobTitle)
		}
		if exp.Company != "" {
			lineParts = append(lineParts, fmt.Sprintf("at %s", exp.Company))
		}
		header := strings.TrimSpace(strings.Join(lineParts, " "))

		desc := strings.TrimSpace(exp.Description)
		if header == "" && desc == "" {
			continue
		}

		if header != "" && desc != "" {
			parts = append(parts, fmt.Sprintf("%s: %s", header, desc))
		} else if header != "" {
			parts = append(parts, header)
		} else {
			parts = append(parts, desc)
		}
	}
	return strings.Join(parts, "\n")
}

func formatMatchEmailBody(userName, recipientEmail string, matches []*models.ResumeJobMatchRecord) string {
	maxCards := 6
	if len(matches) < maxCards {
		maxCards = len(matches)
	}

	greeting := "Hi there,"
	if strings.TrimSpace(userName) != "" {
		greeting = fmt.Sprintf("Hi %s,", htmlEscape(userName))
	}

	brand := getBrandName()
	appURL := htmlEscape(getAppURL())
	support := htmlEscape(getSupportEmail())
	unsubscribeURL := htmlEscape(buildUnsubscribeURL(recipientEmail))

	var cards []string
	for i := 0; i < maxCards; i++ {
		cards = append(cards, renderJobCard(matches[i]))
	}

	jobCount := len(matches)

	return fmt.Sprintf(`
<!doctype html>
<html>
<body style="font-family: 'Segoe UI','Helvetica Neue',Arial,sans-serif; background:#f5f7fb; padding:32px; color:#111827;">
  <div style="max-width:720px; margin:0 auto; background:#ffffff; border:1px solid #e5e7eb; border-radius:16px; box-shadow:0 16px 40px rgba(15,23,42,0.10); overflow:hidden;">
    <div style="padding:18px 22px; background:#0b66c3; color:#fff; display:flex; align-items:center; justify-content:space-between;">
      <div style="font-size:18px; font-weight:700; letter-spacing:0.2px;">%s</div>
      <div style="font-size:12px; opacity:0.9;">Job alert</div>
    </div>
    <div style="padding:22px 24px 14px 24px;">
      <p style="margin:0 0 8px; font-size:14px; color:#4b5563;">%s</p>
      <h2 style="margin:0 0 8px; font-size:20px; font-weight:700; color:#0f172a;">New roles that fit your profile</h2>
      <p style="margin:0 0 18px; font-size:14px; color:#4b5563;">We found %d job%s that match your resume. Here are the highlights:</p>
    </div>
    <div style="padding:0 16px 10px 16px;">%s</div>
    <div style="padding:0 24px 20px 24px; display:flex; gap:10px; flex-wrap:wrap;">
      <a href="%s" style="display:inline-block; padding:12px 18px; background:#0b66c3; color:#fff; text-decoration:none; border-radius:10px; font-weight:700; box-shadow:0 10px 20px rgba(11,102,195,0.25);">See all matches</a>
      <a href="%s" style="display:inline-block; padding:12px 18px; background:#e5e7eb; color:#0f172a; text-decoration:none; border-radius:10px; font-weight:700;">Update my resume</a>
    </div>
    <div style="padding:12px 24px 18px 24px; font-size:12px; color:#6b7280; border-top:1px solid #e5e7eb;">
      <div style="margin-bottom:6px;">You received this because you saved a resume on %s.</div>
      <div style="margin-bottom:6px;">Need help? Contact <a href="mailto:%s" style="color:#0b66c3; text-decoration:none;">%s</a>.</div>
      <div><a href="%s" style="color:#9ca3af; text-decoration:underline;">Unsubscribe</a> from these emails.</div>
    </div>
  </div>
</body>
</html>
`, brand, greeting, jobCount, pluralize(jobCount), strings.Join(cards, ""), appURL, appURL, brand, support, support, unsubscribeURL)
}

func htmlEscape(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&#39;",
	)
	return replacer.Replace(s)
}

func getAppURL() string {
	if v := strings.TrimSpace(os.Getenv("APP_URL")); v != "" {
		return v
	}
	return "https://hihired.org"
}

func getBrandName() string {
	if v := strings.TrimSpace(os.Getenv("APP_BRAND")); v != "" {
		return htmlEscape(v)
	}
	return "HiHired"
}

func getSupportEmail() string {
	if v := strings.TrimSpace(os.Getenv("SUPPORT_EMAIL")); v != "" {
		return v
	}
	return "support@hihired.org"
}

func getUnsubscribeSecret() string {
	secret := strings.TrimSpace(os.Getenv("UNSUBSCRIBE_SECRET"))
	if secret == "" {
		secret = strings.TrimSpace(os.Getenv("JWT_SECRET"))
	}
	if secret == "" {
		secret = "default-unsubscribe-secret-change-me"
	}
	return secret
}

func generateUnsubscribeToken(email string) string {
	secret := getUnsubscribeSecret()
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(strings.ToLower(strings.TrimSpace(email))))
	return hex.EncodeToString(h.Sum(nil))
}

func buildUnsubscribeURL(email string) string {
	base := strings.TrimRight(getAppURL(), "/")
	token := generateUnsubscribeToken(email)
	return fmt.Sprintf("%s/api/email/unsubscribe?email=%s&token=%s", base, url.QueryEscape(email), url.QueryEscape(token))
}

func pluralize(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func renderJobCard(match *models.ResumeJobMatchRecord) string {
	title := htmlEscape(strings.TrimSpace(match.JobTitle))
	company := ""
	if match.CompanyName != nil {
		company = htmlEscape(strings.TrimSpace(*match.CompanyName))
	}
	location := htmlEscape(formatLocation(match.JobLocation, match.JobRemoteType))
	desc := htmlEscape(formatJobDescriptionSnippet(match.JobDescription, 180))
	url := htmlEscape(strings.TrimSpace(match.JobURL))
	tailorURL := htmlEscape(buildTailorURL(match.JobPostingID))

	if title == "" && company == "" {
		title = "Role"
	}

	linkStart := ""
	linkEnd := ""
	if url != "" {
		linkStart = fmt.Sprintf(`<a href="%s" style="color:#0b66c3; text-decoration:none;">`, url)
		linkEnd = `</a>`
	}

	return fmt.Sprintf(`
  <div style="margin:0 0 10px 0; padding:14px 14px 12px 14px; border:1px solid #e5e7eb; border-radius:12px; background:#fff;">
    <div style="display:flex; align-items:flex-start; gap:12px;">
      <div style="flex:1;">
        <div style="font-size:16px; font-weight:700; margin:0 0 4px;">%s%s%s</div>
        <div style="font-size:13px; color:#4b5563; margin:0 0 6px;">%s</div>
        <div style="font-size:13px; color:#6b7280; margin:0 0 8px;">%s</div>
        <a href="%s" style="display:inline-block; padding:8px 12px; background:#0b66c3; color:#fff; text-decoration:none; border-radius:8px; font-weight:700; font-size:13px; box-shadow:0 8px 14px rgba(11,102,195,0.20);">Tailor my resume</a>
      </div>
    </div>
  </div>
`, linkStart, titleWithCompany(title, company), linkEnd, location, desc, tailorURL)
}

func titleWithCompany(title, company string) string {
	if title == "" {
		return company
	}
	if company == "" {
		return title
	}
	return fmt.Sprintf("%s · %s", title, company)
}

func formatLocation(loc, remoteType string) string {
	loc = normalizeWhitespace(loc)
	remote := normalizeWhitespace(remoteType)

	loc = trimDuplicateRemoteLocation(loc)
	if remote != "" && strings.Contains(strings.ToLower(loc), strings.ToLower(remote)) {
		remote = ""
	}

	if loc == "" && remote == "" {
		return "Remote or on-site"
	}
	if loc != "" && remote != "" {
		return fmt.Sprintf("%s · %s", loc, remote)
	}
	if loc != "" {
		return loc
	}
	return remote
}

var (
	htmlScriptStyleRegexp = regexp.MustCompile(`(?is)(<script[^>]*>.*?</script>|<style[^>]*>.*?</style>)`)
	htmlBreakRegexp       = regexp.MustCompile(`(?i)<br\s*/?>`)
	htmlTagRegexp         = regexp.MustCompile(`(?s)<[^>]+>`)
	whitespaceRegexp      = regexp.MustCompile(`\s+`)
)

func formatJobDescriptionSnippet(raw string, max int) string {
	cleaned := strings.TrimSpace(raw)
	if cleaned == "" {
		return ""
	}

	// Some ATS sources store descriptions HTML-escaped (e.g. "&lt;div..."), and
	// occasionally double-escaped (e.g. "&amp;lt;div..."). Decode a few times
	// until stable so tag stripping works reliably.
	for i := 0; i < 3; i++ {
		unescaped := stdhtml.UnescapeString(cleaned)
		if unescaped == cleaned {
			break
		}
		cleaned = unescaped
	}
	cleaned = htmlScriptStyleRegexp.ReplaceAllString(cleaned, " ")
	cleaned = htmlBreakRegexp.ReplaceAllString(cleaned, "\n")
	cleaned = strings.NewReplacer("</p>", "\n", "</div>", "\n", "</li>", "\n", "<li>", " - ").Replace(cleaned)
	cleaned = htmlTagRegexp.ReplaceAllString(cleaned, " ")
	cleaned = normalizeWhitespace(cleaned)

	return truncate(cleaned, max)
}

func normalizeWhitespace(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = whitespaceRegexp.ReplaceAllString(value, " ")
	return strings.TrimSpace(value)
}

func trimDuplicateRemoteLocation(loc string) string {
	loc = normalizeWhitespace(loc)
	if loc == "" {
		return ""
	}

	// Handle common duplication like: "Remote - United States Remote".
	lower := strings.ToLower(loc)
	if strings.HasSuffix(lower, " remote") && strings.Contains(lower[:len(lower)-len(" remote")], "remote") {
		loc = strings.TrimSpace(loc[:len(loc)-len(" remote")])
		loc = strings.TrimRight(strings.TrimSpace(loc), "-·,")
		loc = normalizeWhitespace(loc)
	}
	return loc
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len([]rune(s)) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max-1]) + "…"
}

func buildTailorURL(jobPostingID int64) string {
	base := strings.TrimRight(getAppURL(), "/")
	return fmt.Sprintf("%s/builder?jobId=%d", base, jobPostingID)
}
