package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"resumeai/models"
	"resumeai/utils"
)

type WorkableProvider struct {
	client *http.Client
	logger *utils.Logger
}

type workableJobResponse struct {
	Jobs []workableJob `json:"jobs"`
}

type workableJob struct {
	ID             string             `json:"id"`
	Title          string             `json:"title"`
	Department     string             `json:"department"`
	JobType        string             `json:"job_type"`
	UpdatedAt      string             `json:"updated_at"`
	CreatedAt      string             `json:"created_at"`
	Shortcode      string             `json:"shortcode"`
	URL            string             `json:"url"`
	ApplicationURL string             `json:"application_url"`
	Locations      []workableLocation `json:"locations"`
	Description    string             `json:"description"`
}

type workableLocation struct {
	City    string `json:"city"`
	Region  string `json:"region"`
	Country string `json:"country"`
	Remote  bool   `json:"remote"`
	Text    string `json:"text"`
}

func NewWorkableProvider(client *http.Client, logger *utils.Logger) *WorkableProvider {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	if logger == nil {
		logger = utils.NewLogger()
	}
	return &WorkableProvider{client: client, logger: logger}
}

func (p *WorkableProvider) FetchJobs(ctx context.Context, company *models.JobCompany) ([]*models.JobPosting, error) {
	slug, err := p.resolveCompanySlug(company)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("https://apply.workable.com/api/v3/widget/jobs?company=%s&limit=200", url.QueryEscape(slug))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("workable returned status %d", resp.StatusCode)
	}

	var payload workableJobResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("parse workable response: %w", err)
	}

	results := make([]*models.JobPosting, 0, len(payload.Jobs))
	for _, job := range payload.Jobs {
		posting, err := p.transformJob(company.ID, job)
		if err != nil {
			p.logger.Warn("skipping workable job", map[string]interface{}{"company_id": company.ID, "error": err.Error()})
			continue
		}
		results = append(results, posting)
	}

	return results, nil
}

func (p *WorkableProvider) resolveCompanySlug(company *models.JobCompany) (string, error) {
	if company.ExternalIdentifier != nil && strings.TrimSpace(*company.ExternalIdentifier) != "" {
		return strings.TrimSpace(*company.ExternalIdentifier), nil
	}

	if company.CareersURL == "" {
		return "", fmt.Errorf("workable company slug missing for %s", company.Name)
	}

	parsed, err := url.Parse(company.CareersURL)
	if err != nil {
		return "", fmt.Errorf("invalid careers_url for %s: %w", company.Name, err)
	}

	segments := strings.FieldsFunc(parsed.Path, func(r rune) bool { return r == '/' })
	for _, segment := range segments {
		if segment != "" && segment != "careers" && segment != "jobs" && segment != "list" {
			return strings.TrimSpace(segment), nil
		}
	}

	return "", fmt.Errorf("could not derive workable slug from %s", company.CareersURL)
}

func (p *WorkableProvider) transformJob(companyID int, job workableJob) (*models.JobPosting, error) {
	if strings.TrimSpace(job.ID) == "" || strings.TrimSpace(job.Title) == "" {
		return nil, fmt.Errorf("missing job id or title")
	}

	location := buildWorkableLocation(job.Locations)
	remote := deriveWorkableRemote(job.Locations, location)
	employmentType := strings.TrimSpace(job.JobType)
	department := strings.TrimSpace(job.Department)

	postedAt := parseWorkableTime(job.CreatedAt)
	jobURL := firstNonEmpty(strings.TrimSpace(job.URL))
	applyURL := firstNonEmpty(strings.TrimSpace(job.ApplicationURL), jobURL)

	description := strings.TrimSpace(job.Description)
	if description == "" {
		description = job.Title
	}

	raw, _ := json.Marshal(job)

	return &models.JobPosting{
		CompanyID:      companyID,
		ExternalJobID:  strings.TrimSpace(job.ID),
		Title:          strings.TrimSpace(job.Title),
		Location:       location,
		RemoteType:     remote,
		Department:     department,
		EmploymentType: employmentType,
		JobURL:         jobURL,
		ApplicationURL: applyURL,
		Description:    description,
		PostedAt:       postedAt,
		RawPayload:     raw,
	}, nil
}

func buildWorkableLocation(locations []workableLocation) string {
	if len(locations) == 0 {
		return ""
	}

	loc := locations[0]
	if strings.TrimSpace(loc.Text) != "" {
		return strings.TrimSpace(loc.Text)
	}

	parts := []string{}
	if loc.City != "" {
		parts = append(parts, loc.City)
	}
	if loc.Region != "" {
		parts = append(parts, loc.Region)
	}
	if loc.Country != "" {
		parts = append(parts, loc.Country)
	}
	return strings.Join(parts, ", ")
}

func deriveWorkableRemote(locations []workableLocation, locationStr string) string {
	for _, loc := range locations {
		if loc.Remote {
			return "Remote"
		}
	}
	if strings.Contains(strings.ToLower(locationStr), "remote") {
		return "Remote"
	}
	return ""
}

func parseWorkableTime(value string) *time.Time {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	formats := []string{time.RFC3339, "2006-01-02T15:04:05Z07:00"}
	for _, layout := range formats {
		if ts, err := time.Parse(layout, trimmed); err == nil {
			return &ts
		}
	}
	return nil
}
