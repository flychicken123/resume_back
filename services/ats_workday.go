package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"resumeai/models"
	"resumeai/utils"
)

type WorkdayProvider struct {
	client *http.Client
	logger *utils.Logger
}

type workdayRequest struct {
	Limit         int                 `json:"limit"`
	Offset        int                 `json:"offset"`
	AppliedFacets map[string][]string `json:"appliedFacets"`
	SearchText    string              `json:"searchText"`
}

type workdayJobResponse struct {
	Total       int                 `json:"total"`
	JobPostings []workdayJobPosting `json:"jobPostings"`
}

type workdayJobPosting struct {
	JobPostingID   string             `json:"jobPostingId"`
	Title          string             `json:"title"`
	ExternalPath   string             `json:"externalPath"`
	ExternalURL    string             `json:"externalUrl"`
	TimePosted     string             `json:"timePosted"`
	Locations      []workdayLocation  `json:"locations"`
	JobPostingInfo workdayPostingInfo `json:"jobPostingInfo"`
}

type workdayLocation struct {
	City    string `json:"city"`
	Region  string `json:"region"`
	State   string `json:"state"`
	Country string `json:"country"`
}

type workdayPostingInfo struct {
	JobDescriptions []workdayJobDescription `json:"jobDescriptions"`
	JobTitle        string                  `json:"jobTitle"`
	JobPostingSite  string                  `json:"jobPostingSite"`
	JobLocation     string                  `json:"jobLocation"`
	JobFamily       string                  `json:"jobFamily"`
	JobCategory     string                  `json:"jobCategory"`
	JobType         string                  `json:"jobType"`
}

type workdayJobDescription struct {
	Type string `json:"descriptor"`
	Text string `json:"text"`
}

func NewWorkdayProvider(client *http.Client, logger *utils.Logger) *WorkdayProvider {
	if client == nil {
		client = &http.Client{Timeout: 25 * time.Second}
	}
	if logger == nil {
		logger = utils.NewLogger()
	}
	return &WorkdayProvider{client: client, logger: logger}
}

func (p *WorkdayProvider) FetchJobs(ctx context.Context, company *models.JobCompany) ([]*models.JobPosting, error) {
	endpoint, err := p.buildEndpoint(company)
	if err != nil {
		return nil, err
	}

	const pageSize = 50
	offset := 0
	var postings []*models.JobPosting

	for {
		batch, count, err := p.fetchPage(ctx, company.ID, endpoint, offset, pageSize)
		if err != nil {
			return nil, err
		}
		postings = append(postings, batch...)
		if count < pageSize {
			break
		}
		offset += pageSize
	}

	return postings, nil
}

func (p *WorkdayProvider) buildEndpoint(company *models.JobCompany) (string, error) {
	if company.CareersURL == "" {
		return "", fmt.Errorf("careers_url required for workday company %s", company.Name)
	}

	parsed, err := url.Parse(company.CareersURL)
	if err != nil {
		return "", fmt.Errorf("invalid careers_url for %s: %w", company.Name, err)
	}

	host := parsed.Host
	if host == "" {
		return "", fmt.Errorf("could not determine workday host for %s", company.Name)
	}

	tenant, site := p.extractTenantAndSite(parsed, company)
	if tenant == "" || site == "" {
		return "", fmt.Errorf("could not determine workday tenant/site for %s", company.Name)
	}

	return fmt.Sprintf("https://%s/wday/cxs/%s/%s/jobs", host, tenant, site), nil
}

func (p *WorkdayProvider) extractTenantAndSite(parsed *url.URL, company *models.JobCompany) (string, string) {
	if company.ExternalIdentifier != nil {
		parts := strings.FieldsFunc(strings.TrimSpace(*company.ExternalIdentifier), func(r rune) bool { return r == ':' || r == '/' || r == '|' })
		if len(parts) >= 2 {
			return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		}
	}

	tenant := ""
	if hostParts := strings.Split(parsed.Host, "."); len(hostParts) > 0 {
		tenant = strings.TrimSuffix(hostParts[0], "-wd")
		if strings.Contains(tenant, "wd") {
			tenant = strings.Split(tenant, ".")[0]
		}
		tenant = strings.Split(tenant, "-")[0]
	}

	pathSegments := strings.FieldsFunc(parsed.Path, func(r rune) bool { return r == '/' })
	site := ""
	for i, segment := range pathSegments {
		lower := strings.ToLower(segment)
		if lower == "job" || lower == "jobs" {
			if i > 0 {
				site = pathSegments[i-1]
			}
			break
		}
	}

	if site == "" && len(pathSegments) > 0 {
		for _, segment := range pathSegments {
			if len(segment) > 0 && !strings.Contains(segment, "-") {
				site = segment
				break
			}
		}
	}

	return strings.TrimSpace(tenant), strings.TrimSpace(site)
}

func (p *WorkdayProvider) fetchPage(ctx context.Context, companyID int, endpoint string, offset, limit int) ([]*models.JobPosting, int, error) {
	reqBody := workdayRequest{
		Limit:         limit,
		Offset:        offset,
		AppliedFacets: map[string][]string{},
		SearchText:    "",
	}
	buffer, err := json.Marshal(reqBody)
	if err != nil {
		return nil, 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(buffer))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 WorkdayClient")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("read workday response: %w", err)
	}

	if resp.StatusCode >= 400 {
		snippet := strings.TrimSpace(string(body))
		if len(snippet) > 512 {
			snippet = snippet[:512] + "..."
		}
		if snippet == "" {
			snippet = "<empty body>"
		}
		return nil, 0, fmt.Errorf("workday returned status %d: %s", resp.StatusCode, snippet)
	}

	var payload workdayJobResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, 0, fmt.Errorf("parse workday response: %w", err)
	}

	results := make([]*models.JobPosting, 0, len(payload.JobPostings))
	for _, job := range payload.JobPostings {
		posting, err := p.transformJob(companyID, endpoint, job)
		if err != nil {
			p.logger.Warn("skipping workday job", map[string]interface{}{"company_id": companyID, "error": err.Error()})
			continue
		}
		results = append(results, posting)
	}

	return results, len(payload.JobPostings), nil
}

func (p *WorkdayProvider) transformJob(companyID int, endpoint string, job workdayJobPosting) (*models.JobPosting, error) {
	if strings.TrimSpace(job.JobPostingID) == "" || strings.TrimSpace(job.Title) == "" {
		return nil, fmt.Errorf("missing job id or title")
	}

	location := buildWorkdayLocation(job.Locations)
	remote := deriveWorkdayRemote(location)
	employmentType := job.JobPostingInfo.JobType
	department := job.JobPostingInfo.JobFamily

	postedAt := parseWorkdayTime(job.TimePosted)
	jobURL := firstNonEmpty(strings.TrimSpace(job.ExternalURL), buildWorkdayJobURL(endpoint, job.ExternalPath))
	if jobURL == "" {
		jobURL = strings.TrimSpace(job.ExternalPath)
	}

	description := extractWorkdayDescription(job.JobPostingInfo.JobDescriptions)
	raw, _ := json.Marshal(job)

	return &models.JobPosting{
		CompanyID:      companyID,
		ExternalJobID:  strings.TrimSpace(job.JobPostingID),
		Title:          strings.TrimSpace(job.Title),
		Location:       location,
		RemoteType:     remote,
		Department:     strings.TrimSpace(department),
		EmploymentType: strings.TrimSpace(employmentType),
		JobURL:         jobURL,
		ApplicationURL: jobURL,
		Description:    description,
		PostedAt:       postedAt,
		RawPayload:     raw,
	}, nil
}

func buildWorkdayLocation(locations []workdayLocation) string {
	if len(locations) == 0 {
		return ""
	}
	loc := locations[0]
	parts := []string{}
	if loc.City != "" {
		parts = append(parts, loc.City)
	}
	if loc.State != "" {
		parts = append(parts, loc.State)
	} else if loc.Region != "" {
		parts = append(parts, loc.Region)
	}
	if loc.Country != "" {
		parts = append(parts, loc.Country)
	}
	return strings.Join(parts, ", ")
}

func deriveWorkdayRemote(location string) string {
	if strings.Contains(strings.ToLower(location), "remote") {
		return "Remote"
	}
	return ""
}

func parseWorkdayTime(value string) *time.Time {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	formats := []string{time.RFC3339, "2006-01-02T15:04:05.000Z", "2006-01-02T15:04:05-07:00"}
	for _, layout := range formats {
		if ts, err := time.Parse(layout, trimmed); err == nil {
			return &ts
		}
	}
	return nil
}

func extractWorkdayDescription(descriptions []workdayJobDescription) string {
	for _, desc := range descriptions {
		if strings.Contains(strings.ToLower(desc.Type), "job description") && strings.TrimSpace(desc.Text) != "" {
			return strings.TrimSpace(desc.Text)
		}
	}
	for _, desc := range descriptions {
		if strings.TrimSpace(desc.Text) != "" {
			return strings.TrimSpace(desc.Text)
		}
	}
	return ""
}

func buildWorkdayJobURL(endpoint, externalPath string) string {
	if strings.TrimSpace(externalPath) == "" {
		return ""
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return strings.TrimSpace(externalPath)
	}
	parsed.Path = strings.TrimSpace(externalPath)
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}
