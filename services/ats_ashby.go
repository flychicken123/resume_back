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

type AshbyProvider struct {
	client *http.Client
	logger *utils.Logger
}

type ashbyJobResponse struct {
	JobPostings []ashbyJob `json:"jobPostings"`
}

type ashbyJob struct {
	ID              string           `json:"id"`
	Title           string           `json:"title"`
	JobURL          string           `json:"jobPostingUrl"`
	ApplyURL        string           `json:"applyUrl"`
	CreatedAt       string           `json:"createdAt"`
	Departments     []ashbyLabel     `json:"departments"`
	Teams           []ashbyLabel     `json:"teams"`
	EmploymentTypes []ashbyLabel     `json:"employmentTypes"`
	Sections        []ashbySection   `json:"sections"`
	JobLocation     ashbyJobLocation `json:"jobLocation"`
}

type ashbyLabel struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}

type ashbySection struct {
	Title    string `json:"title"`
	BodyHTML string `json:"bodyHtml"`
	BodyText string `json:"bodyText"`
	Type     string `json:"sectionType"`
}

type ashbyJobLocation struct {
	IsRemote bool         `json:"isRemote"`
	Address  ashbyAddress `json:"address"`
}

type ashbyAddress struct {
	City    string `json:"city"`
	Region  string `json:"region"`
	Country string `json:"country"`
}

func NewAshbyProvider(client *http.Client, logger *utils.Logger) *AshbyProvider {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	if logger == nil {
		logger = utils.NewLogger()
	}
	return &AshbyProvider{client: client, logger: logger}
}

func (p *AshbyProvider) FetchJobs(ctx context.Context, company *models.JobCompany) ([]*models.JobPosting, error) {
	slug, err := p.resolveOrgSlug(company)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("https://jobs.ashbyhq.com/api/org/%s/job-board/job-posting", url.PathEscape(slug))
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
		return nil, fmt.Errorf("ashby returned status %d", resp.StatusCode)
	}

	var payload ashbyJobResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("parse ashby response: %w", err)
	}

	results := make([]*models.JobPosting, 0, len(payload.JobPostings))
	for _, job := range payload.JobPostings {
		posting, err := p.transformJob(company.ID, job)
		if err != nil {
			p.logger.Warn("skipping ashby job", map[string]interface{}{"company_id": company.ID, "error": err.Error()})
			continue
		}
		results = append(results, posting)
	}

	return results, nil
}

func (p *AshbyProvider) resolveOrgSlug(company *models.JobCompany) (string, error) {
	if company.ExternalIdentifier != nil && strings.TrimSpace(*company.ExternalIdentifier) != "" {
		return strings.TrimSpace(*company.ExternalIdentifier), nil
	}

	if company.CareersURL == "" {
		return "", fmt.Errorf("ashby org slug missing for %s", company.Name)
	}

	parsed, err := url.Parse(company.CareersURL)
	if err != nil {
		return "", fmt.Errorf("invalid careers_url for %s: %w", company.Name, err)
	}

	segments := strings.FieldsFunc(parsed.Path, func(r rune) bool { return r == '/' })
	for _, segment := range segments {
		if strings.EqualFold(segment, "jobs") {
			continue
		}
		if segment != "" {
			return strings.TrimSpace(segment), nil
		}
	}

	return "", fmt.Errorf("could not derive ashby org slug from %s", company.CareersURL)
}

func (p *AshbyProvider) transformJob(companyID int, job ashbyJob) (*models.JobPosting, error) {
	if strings.TrimSpace(job.ID) == "" || strings.TrimSpace(job.Title) == "" {
		return nil, fmt.Errorf("missing job id or title")
	}

	location := buildAshbyLocation(job.JobLocation)
	remote := deriveAshbyRemote(job.JobLocation, location)
	employmentType := collectAshbyLabel(job.EmploymentTypes)
	department := collectAshbyLabel(job.Departments)
	if department == "" {
		department = collectAshbyLabel(job.Teams)
	}

	postedAt := parseAshbyTime(job.CreatedAt)
	jobURL := firstNonEmpty(strings.TrimSpace(job.JobURL), buildAshbyJobURL(job.ID))
	applyURL := firstNonEmpty(strings.TrimSpace(job.ApplyURL), jobURL)

	description := extractAshbyDescription(job.Sections)
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

func buildAshbyLocation(loc ashbyJobLocation) string {
	parts := []string{}
	if loc.Address.City != "" {
		parts = append(parts, loc.Address.City)
	}
	if loc.Address.Region != "" {
		parts = append(parts, loc.Address.Region)
	}
	if loc.Address.Country != "" {
		parts = append(parts, loc.Address.Country)
	}
	return strings.Join(parts, ", ")
}

func deriveAshbyRemote(loc ashbyJobLocation, location string) string {
	if loc.IsRemote {
		return "Remote"
	}
	if strings.Contains(strings.ToLower(location), "remote") {
		return "Remote"
	}
	return ""
}

func collectAshbyLabel(labels []ashbyLabel) string {
	for _, label := range labels {
		if strings.TrimSpace(label.Label) != "" {
			return strings.TrimSpace(label.Label)
		}
		if strings.TrimSpace(label.Name) != "" {
			return strings.TrimSpace(label.Name)
		}
	}
	return ""
}

func extractAshbyDescription(sections []ashbySection) string {
	for _, section := range sections {
		if strings.TrimSpace(section.BodyText) != "" {
			return strings.TrimSpace(section.BodyText)
		}
		if strings.TrimSpace(section.BodyHTML) != "" {
			return strings.TrimSpace(section.BodyHTML)
		}
	}
	return ""
}

func parseAshbyTime(value string) *time.Time {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	formats := []string{time.RFC3339, "2006-01-02T15:04:05.999Z"}
	for _, layout := range formats {
		if ts, err := time.Parse(layout, trimmed); err == nil {
			return &ts
		}
	}
	return nil
}

func buildAshbyJobURL(id string) string {
	return fmt.Sprintf("https://jobs.ashbyhq.com/job/%s", url.PathEscape(id))
}
