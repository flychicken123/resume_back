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

type RecruiteeProvider struct {
	client *http.Client
	logger *utils.Logger
}

type recruiteeResponse struct {
	Offers []recruiteeOffer `json:"offers"`
}

type recruiteeOffer struct {
	ID             int       `json:"id"`
	Slug           string    `json:"slug"`
	Title          string    `json:"title"`
	Description    string    `json:"description"`
	Department     string    `json:"department"`
	Location       string    `json:"location"`
	City           string    `json:"city"`
	Country        string    `json:"country"`
	Remote         bool      `json:"remote"`
	EmploymentType string    `json:"employment_type_code"`
	CareersURL     string    `json:"careers_url"`
	CreatedAt      time.Time `json:"created_at"`
	PublishedAt    time.Time `json:"published_at"`
}

func NewRecruiteeProvider(client *http.Client, logger *utils.Logger) *RecruiteeProvider {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	if logger == nil {
		logger = utils.NewLogger()
	}
	return &RecruiteeProvider{client: client, logger: logger}
}

func (p *RecruiteeProvider) FetchJobs(ctx context.Context, company *models.JobCompany) ([]*models.JobPosting, error) {
	slug, err := p.resolveCompanySlug(company)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("https://%s.recruitee.com/api/offers", url.PathEscape(slug))
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
			p.logger.Warn("recruitee company not found, returning no jobs", map[string]interface{}{"slug": slug, "company_id": company.ID})
			return []*models.JobPosting{}, nil
		}
		return nil, fmt.Errorf("recruitee returned status %d for slug %s", resp.StatusCode, slug)
	}

	var payload recruiteeResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("parse recruitee response: %w", err)
	}

	results := make([]*models.JobPosting, 0, len(payload.Offers))
	for _, offer := range payload.Offers {
		posting, err := p.transformOffer(company.ID, slug, offer)
		if err != nil {
			p.logger.Warn("skipping recruitee offer", map[string]interface{}{"company_id": company.ID, "error": err.Error()})
			continue
		}
		results = append(results, posting)
	}

	return results, nil
}

func (p *RecruiteeProvider) resolveCompanySlug(company *models.JobCompany) (string, error) {
	if company.ExternalIdentifier != nil && strings.TrimSpace(*company.ExternalIdentifier) != "" {
		return strings.TrimSpace(*company.ExternalIdentifier), nil
	}

	if company.CareersURL == "" {
		return "", fmt.Errorf("recruitee company slug missing for %s", company.Name)
	}

	parsed, err := url.Parse(company.CareersURL)
	if err != nil {
		return "", fmt.Errorf("invalid careers_url for %s: %w", company.Name, err)
	}

	// Extract subdomain from host like "company.recruitee.com"
	host := strings.ToLower(parsed.Host)
	parts := strings.Split(host, ".")
	if len(parts) >= 3 && strings.Contains(host, "recruitee.com") {
		slug := strings.TrimSpace(parts[0])
		if slug != "" && slug != "www" {
			return slug, nil
		}
	}

	return "", fmt.Errorf("could not derive recruitee slug from %s", company.CareersURL)
}

func (p *RecruiteeProvider) transformOffer(companyID int, companySlug string, offer recruiteeOffer) (*models.JobPosting, error) {
	if offer.ID == 0 || strings.TrimSpace(offer.Title) == "" {
		return nil, fmt.Errorf("missing offer id or title")
	}

	externalID := fmt.Sprintf("%d", offer.ID)
	location := p.buildLocation(offer)
	remoteType := ""
	if offer.Remote {
		remoteType = "Remote"
	}

	jobURL := strings.TrimSpace(offer.CareersURL)
	if jobURL == "" {
		jobURL = fmt.Sprintf("https://%s.recruitee.com/o/%s", companySlug, offer.Slug)
	}

	var postedAt *time.Time
	if !offer.PublishedAt.IsZero() {
		postedAt = &offer.PublishedAt
	} else if !offer.CreatedAt.IsZero() {
		postedAt = &offer.CreatedAt
	}

	raw, _ := json.Marshal(offer)

	return &models.JobPosting{
		CompanyID:      companyID,
		ExternalJobID:  externalID,
		Title:          strings.TrimSpace(offer.Title),
		Location:       location,
		RemoteType:     remoteType,
		Department:     strings.TrimSpace(offer.Department),
		EmploymentType: p.mapEmploymentType(offer.EmploymentType),
		JobURL:         jobURL,
		ApplicationURL: jobURL,
		Description:    strings.TrimSpace(offer.Description),
		PostedAt:       postedAt,
		RawPayload:     raw,
	}, nil
}

func (p *RecruiteeProvider) buildLocation(offer recruiteeOffer) string {
	if loc := strings.TrimSpace(offer.Location); loc != "" {
		return loc
	}
	parts := []string{}
	if city := strings.TrimSpace(offer.City); city != "" {
		parts = append(parts, city)
	}
	if country := strings.TrimSpace(offer.Country); country != "" {
		parts = append(parts, country)
	}
	return strings.Join(parts, ", ")
}

func (p *RecruiteeProvider) mapEmploymentType(code string) string {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "full_time", "fulltime":
		return "Full-time"
	case "part_time", "parttime":
		return "Part-time"
	case "contract":
		return "Contract"
	case "internship":
		return "Internship"
	case "freelance":
		return "Freelance"
	default:
		return code
	}
}
