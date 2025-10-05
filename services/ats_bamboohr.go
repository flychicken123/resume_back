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

type BambooHRProvider struct {
	client *http.Client
	logger *utils.Logger
}

type bambooHRJobResponse struct {
	Success bool          `json:"success"`
	Jobs    []bambooHRJob `json:"jobs"`
}

type bambooHRJob struct {
	ID                interface{}      `json:"id"`
	JobOpeningName    string           `json:"jobOpeningName"`
	Department        string           `json:"department"`
	JobType           string           `json:"workType"`
	Location          bambooHRLocation `json:"location"`
	JobURL            string           `json:"jobUrl"`
	Remote            bool             `json:"remote"`
	WorkplaceType     string           `json:"workplaceType"`
	Description       string           `json:"jobDescription"`
	DatePosted        string           `json:"datePosted"`
	DateLastPublished string           `json:"dateLastPublished"`
}

type bambooHRLocation struct {
	Location string `json:"name"`
	City     string `json:"city"`
	State    string `json:"state"`
	Country  string `json:"country"`
}

func NewBambooHRProvider(client *http.Client, logger *utils.Logger) *BambooHRProvider {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	if logger == nil {
		logger = utils.NewLogger()
	}
	return &BambooHRProvider{client: client, logger: logger}
}

func (p *BambooHRProvider) FetchJobs(ctx context.Context, company *models.JobCompany) ([]*models.JobPosting, error) {
	subdomain, err := p.resolveSubdomain(company)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("https://%s.bamboohr.com/careers/jobs?format=json", subdomain)
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
		return nil, fmt.Errorf("bamboohr returned status %d", resp.StatusCode)
	}

	var payload bambooHRJobResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("parse bamboohr response: %w", err)
	}

	results := make([]*models.JobPosting, 0, len(payload.Jobs))
	for _, job := range payload.Jobs {
		posting, err := p.transformJob(company.ID, subdomain, job)
		if err != nil {
			p.logger.Warn("skipping bamboohr job", map[string]interface{}{"company_id": company.ID, "error": err.Error()})
			continue
		}
		results = append(results, posting)
	}

	return results, nil
}

func (p *BambooHRProvider) resolveSubdomain(company *models.JobCompany) (string, error) {
	if company.ExternalIdentifier != nil && strings.TrimSpace(*company.ExternalIdentifier) != "" {
		return strings.TrimSpace(*company.ExternalIdentifier), nil
	}

	if company.CareersURL == "" {
		return "", fmt.Errorf("bamboohr subdomain missing for %s", company.Name)
	}

	parsed, err := url.Parse(company.CareersURL)
	if err != nil {
		return "", fmt.Errorf("invalid careers_url for %s: %w", company.Name, err)
	}

	hostParts := strings.Split(parsed.Host, ".")
	if len(hostParts) == 0 {
		return "", fmt.Errorf("could not derive bamboohr subdomain from %s", company.CareersURL)
	}

	return strings.TrimSpace(hostParts[0]), nil
}

func (p *BambooHRProvider) transformJob(companyID int, subdomain string, job bambooHRJob) (*models.JobPosting, error) {
	if strings.TrimSpace(job.JobOpeningName) == "" {
		return nil, fmt.Errorf("missing job title")
	}

	externalID := fmt.Sprint(job.ID)
	location := buildBambooHRLocation(job.Location)
	remote := deriveBambooHRRemote(job, location)
	employmentType := strings.TrimSpace(job.JobType)
	department := strings.TrimSpace(job.Department)

	jobURL := firstNonEmpty(strings.TrimSpace(job.JobURL), fmt.Sprintf("https://%s.bamboohr.com/careers/%s", subdomain, externalID))

	description := strings.TrimSpace(job.Description)
	if description == "" {
		description = job.JobOpeningName
	}

	postedAt := parseBambooHRTime(firstNonEmpty(job.DatePosted, job.DateLastPublished))
	raw, _ := json.Marshal(job)

	return &models.JobPosting{
		CompanyID:      companyID,
		ExternalJobID:  externalID,
		Title:          strings.TrimSpace(job.JobOpeningName),
		Location:       location,
		RemoteType:     remote,
		Department:     department,
		EmploymentType: employmentType,
		JobURL:         jobURL,
		ApplicationURL: jobURL,
		Description:    description,
		PostedAt:       postedAt,
		RawPayload:     raw,
	}, nil
}

func buildBambooHRLocation(loc bambooHRLocation) string {
	if strings.TrimSpace(loc.Location) != "" {
		return strings.TrimSpace(loc.Location)
	}
	parts := []string{}
	if loc.City != "" {
		parts = append(parts, loc.City)
	}
	if loc.State != "" {
		parts = append(parts, loc.State)
	}
	if loc.Country != "" {
		parts = append(parts, loc.Country)
	}
	return strings.Join(parts, ", ")
}

func deriveBambooHRRemote(job bambooHRJob, location string) string {
	if job.Remote || strings.EqualFold(job.WorkplaceType, "remote") {
		return "Remote"
	}
	if strings.Contains(strings.ToLower(location), "remote") {
		return "Remote"
	}
	return ""
}

func parseBambooHRTime(value string) *time.Time {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	formats := []string{"2006-01-02", time.RFC3339}
	for _, layout := range formats {
		if ts, err := time.Parse(layout, trimmed); err == nil {
			return &ts
		}
	}
	return nil
}
