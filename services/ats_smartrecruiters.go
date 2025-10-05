package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"resumeai/models"
	"resumeai/utils"
)

type SmartRecruitersProvider struct {
	client *http.Client
	logger *utils.Logger
}

type smartRecruitersResponse struct {
	TotalFound int                  `json:"totalFound"`
	Content    []smartRecruitersJob `json:"content"`
}

type smartRecruitersJob struct {
	ID           string                  `json:"id"`
	Name         string                  `json:"name"`
	RefNumber    string                  `json:"refNumber"`
	ReleasedDate string                  `json:"releasedDate"`
	Location     smartRecruitersLocation `json:"location"`
	Department   smartRecruitersLabel    `json:"department"`
	Function     smartRecruitersLabel    `json:"function"`
	Type         smartRecruitersLabel    `json:"type"`
	ApplyURL     string                  `json:"applyUrl"`
	JobAd        smartRecruitersJobAd    `json:"jobAd"`
}

type smartRecruitersLocation struct {
	City    string `json:"city"`
	Region  string `json:"region"`
	Country string `json:"country"`
	Remote  bool   `json:"remote"`
}

type smartRecruitersLabel struct {
	Label string `json:"label"`
}

type smartRecruitersJobAd struct {
	Sections    []smartRecruitersSection `json:"sections"`
	ExternalURL string                   `json:"externalUrl"`
	LandingPage string                   `json:"landingPage"`
	Description string                   `json:"jobDescription"`
}

type smartRecruitersSection struct {
	Title string `json:"title"`
	Text  string `json:"text"`
	Type  string `json:"sectionType"`
}

func NewSmartRecruitersProvider(client *http.Client, logger *utils.Logger) *SmartRecruitersProvider {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	if logger == nil {
		logger = utils.NewLogger()
	}
	return &SmartRecruitersProvider{client: client, logger: logger}
}

func (p *SmartRecruitersProvider) FetchJobs(ctx context.Context, company *models.JobCompany) ([]*models.JobPosting, error) {
	slug, err := p.resolveCompanySlug(company)
	if err != nil {
		return nil, err
	}

	const pageSize = 100
	offset := 0
	var postings []*models.JobPosting

	for {
		batch, count, err := p.fetchPage(ctx, company.ID, slug, offset, pageSize)
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

func (p *SmartRecruitersProvider) resolveCompanySlug(company *models.JobCompany) (string, error) {
	if company.ExternalIdentifier != nil && strings.TrimSpace(*company.ExternalIdentifier) != "" {
		return strings.TrimSpace(*company.ExternalIdentifier), nil
	}

	if company.CareersURL == "" {
		return "", fmt.Errorf("smartrecruiters company slug missing for %s", company.Name)
	}

	parsed, err := url.Parse(company.CareersURL)
	if err != nil {
		return "", fmt.Errorf("invalid careers_url for %s: %w", company.Name, err)
	}

	segments := strings.FieldsFunc(parsed.Path, func(r rune) bool { return r == '/' })
	if len(segments) == 0 {
		return "", fmt.Errorf("could not derive smartrecruiters slug from %s", company.CareersURL)
	}

	// Heuristic: slug is usually first path segment
	slug := strings.TrimSpace(segments[0])
	if slug == "company" && len(segments) > 1 {
		slug = strings.TrimSpace(segments[1])
	}
	if slug == "" {
		return "", fmt.Errorf("could not derive smartrecruiters slug from %s", company.CareersURL)
	}
	return slug, nil
}

func (p *SmartRecruitersProvider) fetchPage(ctx context.Context, companyID int, slug string, offset, limit int) ([]*models.JobPosting, int, error) {
	endpoint := fmt.Sprintf("https://api.smartrecruiters.com/v1/companies/%s/postings", url.PathEscape(slug))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, err
	}

	query := req.URL.Query()
	query.Set("offset", strconv.Itoa(offset))
	query.Set("limit", strconv.Itoa(limit))
	req.URL.RawQuery = query.Encode()

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, 0, fmt.Errorf("smartrecruiters returned status %d", resp.StatusCode)
	}

	var payload smartRecruitersResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, 0, fmt.Errorf("parse smartrecruiters response: %w", err)
	}

	results := make([]*models.JobPosting, 0, len(payload.Content))
	for _, job := range payload.Content {
		posting, err := p.transformJob(companyID, job)
		if err != nil {
			p.logger.Warn("skipping smartrecruiters job", map[string]interface{}{"company_id": companyID, "error": err.Error()})
			continue
		}
		results = append(results, posting)
	}

	return results, len(payload.Content), nil
}

func (p *SmartRecruitersProvider) transformJob(companyID int, job smartRecruitersJob) (*models.JobPosting, error) {
	if strings.TrimSpace(job.ID) == "" || strings.TrimSpace(job.Name) == "" {
		return nil, fmt.Errorf("missing job id or title")
	}

	location := buildSmartRecruitersLocation(job.Location)
	remote := deriveSmartRecruitersRemote(job.Location, location)
	department := strings.TrimSpace(job.Department.Label)
	employmentType := strings.TrimSpace(job.Type.Label)

	postedAt := parseSmartRecruitersTime(job.ReleasedDate)
	jobURL := firstNonEmpty(job.JobAd.ExternalURL, job.JobAd.LandingPage, buildSmartRecruitersJobURL(job.ID))
	applyURL := strings.TrimSpace(job.ApplyURL)
	if applyURL == "" {
		applyURL = jobURL
	}

	description := extractSmartRecruitersDescription(job.JobAd)
	raw, _ := json.Marshal(job)

	return &models.JobPosting{
		CompanyID:      companyID,
		ExternalJobID:  strings.TrimSpace(job.ID),
		Title:          strings.TrimSpace(job.Name),
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

func buildSmartRecruitersLocation(loc smartRecruitersLocation) string {
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

func deriveSmartRecruitersRemote(loc smartRecruitersLocation, locationString string) string {
	if loc.Remote {
		return "Remote"
	}
	if strings.Contains(strings.ToLower(locationString), "remote") {
		return "Remote"
	}
	return ""
}

func parseSmartRecruitersTime(value string) *time.Time {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	formats := []string{time.RFC3339, "2006-01-02T15:04:05-0700"}
	for _, layout := range formats {
		if ts, err := time.Parse(layout, trimmed); err == nil {
			return &ts
		}
	}
	return nil
}

func extractSmartRecruitersDescription(ad smartRecruitersJobAd) string {
	if strings.TrimSpace(ad.Description) != "" {
		return strings.TrimSpace(ad.Description)
	}
	for _, section := range ad.Sections {
		if strings.EqualFold(section.Type, "JOB_DESCRIPTION") || strings.Contains(strings.ToLower(section.Title), "description") {
			if strings.TrimSpace(section.Text) != "" {
				return strings.TrimSpace(section.Text)
			}
		}
	}
	return ""
}

func buildSmartRecruitersJobURL(id string) string {
	return fmt.Sprintf("https://jobs.smartrecruiters.com/%s", path.Clean(id))
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
