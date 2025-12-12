package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
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

	n.runOnce(ctx)

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

		resume := item.Resume
		if strings.TrimSpace(item.UserEmail) == "" {
			continue
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
			CandidateJobLimit: 200,
			MaxResults:        10,
		}

		matches, err := n.matcher.MatchAndStore(ctx, matchInput)
		if err != nil {
			n.logger.Warn("job match notifier: match failed", map[string]interface{}{"user_id": resume.UserID, "error": err.Error()})
			continue
		}
		if len(matches) == 0 {
			continue
		}

		body := formatMatchEmailBody(item.UserName, matches)
		if err := n.email.SendHTMLEmail(item.UserEmail, "New job matches for your resume", body); err != nil {
			n.logger.Warn("job match notifier: email failed", map[string]interface{}{"user_id": resume.UserID, "error": err.Error()})
		} else {
			n.logger.Info("job match notifier: email sent", map[string]interface{}{"user_id": resume.UserID, "matches": len(matches)})
		}
	}
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

func formatMatchEmailBody(userName string, matches []*models.ResumeJobMatchRecord) string {
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
      <div>Need help? Contact <a href="mailto:%s" style="color:#0b66c3; text-decoration:none;">%s</a>.</div>
    </div>
  </div>
</body>
</html>
`, brand, greeting, jobCount, pluralize(jobCount), strings.Join(cards, ""), appURL, appURL, brand, support, support)
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
	desc := htmlEscape(truncate(match.JobDescription, 180))
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
	loc = strings.TrimSpace(loc)
	remote := strings.TrimSpace(remoteType)
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
