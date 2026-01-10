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

type TeamtailorProvider struct {
	client *http.Client
	logger *utils.Logger
}

type teamtailorResponse struct {
	Data []teamtailorJob `json:"data"`
}

type teamtailorJob struct {
	ID         string                   `json:"id"`
	Type       string                   `json:"type"`
	Attributes teamtailorJobAttributes  `json:"attributes"`
	Links      teamtailorLinks          `json:"links"`
}

type teamtailorJobAttributes struct {
	Title          string    `json:"title"`
	Body           string    `json:"body"`
	Pitch          string    `json:"pitch"`
	Department     string    `json:"department"`
	Remote         string    `json:"remote-status"`
	EmploymentType string    `json:"employment-type"`
	CreatedAt      time.Time `json:"created-at"`
	UpdatedAt      time.Time `json:"updated-at"`

	// Location fields
	City    string `json:"city"`
	Country string `json:"country"`
	Region  string `json:"region"`
}

type teamtailorLinks struct {
	CareerSiteJobURL string `json:"careersite-job-url"`
	CareerSiteApply  string `json:"careersite-job-apply-url"`
	Self             string `json:"self"`
}

func NewTeamtailorProvider(client *http.Client, logger *utils.Logger) *TeamtailorProvider {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	if logger == nil {
		logger = utils.NewLogger()
	}
	return &TeamtailorProvider{client: client, logger: logger}
}

func (p *TeamtailorProvider) FetchJobs(ctx context.Context, company *models.JobCompany) ([]*models.JobPosting, error) {
	baseURL, err := p.resolveBaseURL(company)
	if err != nil {
		return nil, err
	}

	// Teamtailor public API endpoint
	endpoint := fmt.Sprintf("%s/api/v1/jobs", strings.TrimSuffix(baseURL, "/"))
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
		if resp.StatusCode == http.StatusNotFound {
			p.logger.Warn("teamtailor endpoint not found, returning no jobs", map[string]interface{}{"url": endpoint, "company_id": company.ID})
			return []*models.JobPosting{}, nil
		}
		return nil, fmt.Errorf("teamtailor returned status %d for %s", resp.StatusCode, endpoint)
	}

	var payload teamtailorResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("parse teamtailor response: %w", err)
	}

	results := make([]*models.JobPosting, 0, len(payload.Data))
	for _, job := range payload.Data {
		posting, err := p.transformJob(company.ID, job)
		if err != nil {
			p.logger.Warn("skipping teamtailor job", map[string]interface{}{"company_id": company.ID, "error": err.Error()})
			continue
		}
		results = append(results, posting)
	}

	return results, nil
}

func (p *TeamtailorProvider) resolveBaseURL(company *models.JobCompany) (string, error) {
	if company.CareersURL == "" {
		return "", fmt.Errorf("teamtailor careers_url missing for %s", company.Name)
	}

	parsed, err := url.Parse(company.CareersURL)
	if err != nil {
		return "", fmt.Errorf("invalid careers_url for %s: %w", company.Name, err)
	}

	// Return the base URL (scheme + host)
	return fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host), nil
}

func (p *TeamtailorProvider) transformJob(companyID int, job teamtailorJob) (*models.JobPosting, error) {
	if strings.TrimSpace(job.ID) == "" || strings.TrimSpace(job.Attributes.Title) == "" {
		return nil, fmt.Errorf("missing job id or title")
	}

	location := p.buildLocation(job.Attributes)
	remoteType := p.mapRemoteStatus(job.Attributes.Remote)

	jobURL := strings.TrimSpace(job.Links.CareerSiteJobURL)
	if jobURL == "" {
		jobURL = strings.TrimSpace(job.Links.Self)
	}

	applyURL := strings.TrimSpace(job.Links.CareerSiteApply)
	if applyURL == "" {
		applyURL = jobURL
	}

	description := strings.TrimSpace(job.Attributes.Body)
	if description == "" {
		description = strings.TrimSpace(job.Attributes.Pitch)
	}

	var postedAt *time.Time
	if !job.Attributes.CreatedAt.IsZero() {
		postedAt = &job.Attributes.CreatedAt
	}

	raw, _ := json.Marshal(job)

	return &models.JobPosting{
		CompanyID:      companyID,
		ExternalJobID:  strings.TrimSpace(job.ID),
		Title:          strings.TrimSpace(job.Attributes.Title),
		Location:       location,
		RemoteType:     remoteType,
		Department:     strings.TrimSpace(job.Attributes.Department),
		EmploymentType: p.mapEmploymentType(job.Attributes.EmploymentType),
		JobURL:         jobURL,
		ApplicationURL: applyURL,
		Description:    description,
		PostedAt:       postedAt,
		RawPayload:     raw,
	}, nil
}

func (p *TeamtailorProvider) buildLocation(attrs teamtailorJobAttributes) string {
	parts := []string{}
	if city := strings.TrimSpace(attrs.City); city != "" {
		parts = append(parts, city)
	}
	if region := strings.TrimSpace(attrs.Region); region != "" {
		parts = append(parts, region)
	}
	if country := strings.TrimSpace(attrs.Country); country != "" {
		parts = append(parts, country)
	}
	return strings.Join(parts, ", ")
}

func (p *TeamtailorProvider) mapRemoteStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "fully", "fully_remote", "remote":
		return "Remote"
	case "hybrid":
		return "Hybrid"
	case "none", "onsite", "on-site":
		return "On-site"
	default:
		if strings.Contains(strings.ToLower(status), "remote") {
			return "Remote"
		}
		return ""
	}
}

func (p *TeamtailorProvider) mapEmploymentType(empType string) string {
	switch strings.ToLower(strings.TrimSpace(empType)) {
	case "full-time", "full_time", "fulltime":
		return "Full-time"
	case "part-time", "part_time", "parttime":
		return "Part-time"
	case "contract", "contractor":
		return "Contract"
	case "internship", "intern":
		return "Internship"
	case "freelance":
		return "Freelance"
	case "temporary":
		return "Temporary"
	default:
		return empType
	}
}
