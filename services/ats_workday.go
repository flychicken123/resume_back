package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"resumeai/models"
	"resumeai/utils"
)

type WorkdayProvider struct {
	client         *http.Client
	logger         *utils.Logger
	userAgent      string
	sessionCookies sync.Map
}

const (
	workdayClientHeader = "job-search"
	defaultLocaleHeader = "en-US,en;q=0.9"
	defaultUserAgent    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/118.0.0.0 Safari/537.36"
)

var (
	workdayHostSuffixRx  = regexp.MustCompile(`(?i)(-wd\d+|wd\d+)$`)
	workdayLocaleSegment = regexp.MustCompile(`(?i)^[a-z]{2}[-_][a-z]{2}$`)
)

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
		jar, _ := cookiejar.New(nil)
		client = &http.Client{
			Timeout: 25 * time.Second,
			Jar:     jar,
		}
	} else if client.Jar == nil {
		if jar, err := cookiejar.New(nil); err == nil {
			client.Jar = jar
		}
	}
	if logger == nil {
		logger = utils.NewLogger()
	}
	return &WorkdayProvider{
		client:    client,
		logger:    logger,
		userAgent: defaultUserAgent,
	}
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
		batch, count, err := p.fetchPage(ctx, company, endpoint, offset, pageSize)
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

	if company.ExternalIdentifier != nil {
		if raw := strings.TrimSpace(*company.ExternalIdentifier); strings.HasPrefix(strings.ToLower(raw), "http://") || strings.HasPrefix(strings.ToLower(raw), "https://") {
			if parsed, err := url.Parse(raw); err == nil && parsed.Host != "" {
				return p.endpointFromURL(parsed, company)
			}
		}
	}

	parsed, err := url.Parse(company.CareersURL)
	if err != nil {
		return "", fmt.Errorf("invalid careers_url for %s: %w", company.Name, err)
	}
	if parsed.Host == "" {
		if withScheme, err := url.Parse("https://" + strings.TrimSpace(company.CareersURL)); err == nil {
			parsed = withScheme
		}
	}

	if resolved, err := p.resolveRedirectTarget(company, parsed); err == nil && resolved != nil {
		parsed = resolved
	}

	return p.endpointFromURL(parsed, company)
}

func (p *WorkdayProvider) endpointFromURL(parsed *url.URL, company *models.JobCompany) (string, error) {
	host := strings.TrimSpace(parsed.Host)
	if host == "" {
		return "", fmt.Errorf("could not determine workday host for %s", company.Name)
	}

	tenant, site := p.extractTenantAndSite(parsed, company)
	if tenant == "" || site == "" {
		return "", fmt.Errorf("could not determine workday tenant/site for %s", company.Name)
	}

	return fmt.Sprintf("https://%s/wday/cxs/%s/%s/jobs", host, tenant, site), nil
}

func (p *WorkdayProvider) resolveRedirectTarget(company *models.JobCompany, parsed *url.URL) (*url.URL, error) {
	if parsed == nil || parsed.Host == "" {
		return nil, fmt.Errorf("invalid url")
	}
	lowerHost := strings.ToLower(parsed.Host)
	if strings.Contains(lowerHost, "myworkdayjobs.com") || strings.Contains(lowerHost, "workday") {
		return nil, fmt.Errorf("already a workday host")
	}

	req, err := http.NewRequest(http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	p.setCommonHeaders(req, parsed.String(), company, false)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

	finalURL := resp.Request.URL
	if finalURL == nil || finalURL.Host == "" {
		return nil, fmt.Errorf("no final url")
	}
	finalHost := strings.ToLower(finalURL.Host)
	if strings.Contains(finalHost, "myworkdayjobs.com") || strings.Contains(finalHost, "workday") {
		return finalURL, nil
	}
	return nil, fmt.Errorf("redirect target not workday")
}

func (p *WorkdayProvider) extractTenantAndSite(parsed *url.URL, company *models.JobCompany) (string, string) {
	if company.ExternalIdentifier != nil {
		parts := strings.FieldsFunc(strings.TrimSpace(*company.ExternalIdentifier), func(r rune) bool { return r == ':' || r == '/' || r == '|' })
		if len(parts) >= 2 {
			return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		}
	}

	pathSegments := strings.FieldsFunc(parsed.Path, func(r rune) bool { return r == '/' })
	for i := 0; i+4 < len(pathSegments); i++ {
		if strings.EqualFold(pathSegments[i], "wday") && strings.EqualFold(pathSegments[i+1], "cxs") {
			tenant := strings.TrimSpace(pathSegments[i+2])
			site := strings.TrimSpace(pathSegments[i+3])
			if tenant != "" && site != "" {
				return tenant, site
			}
		}
	}

	tenant := ""
	if hostParts := strings.Split(parsed.Host, "."); len(hostParts) > 0 {
		tenant = strings.TrimSpace(hostParts[0])
		tenant = workdayHostSuffixRx.ReplaceAllString(tenant, "")
		tenant = strings.TrimSuffix(tenant, "-")
	}

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
		for i := len(pathSegments) - 1; i >= 0; i-- {
			segment := strings.TrimSpace(pathSegments[i])
			if segment == "" {
				continue
			}
			if workdayLocaleSegment.MatchString(segment) {
				continue
			}
			if strings.EqualFold(segment, "global") {
				continue
			}
			site = segment
			break
		}
	}

	return strings.TrimSpace(tenant), strings.TrimSpace(site)
}

func (p *WorkdayProvider) fetchPage(ctx context.Context, company *models.JobCompany, endpoint string, offset, limit int) ([]*models.JobPosting, int, error) {
	cookieHeader, err := p.warmUpSession(ctx, endpoint, company)
	if err != nil {
		p.logger.Warn("workday warmup failed; continuing without cookies", map[string]interface{}{
			"company_id": func() int {
				if company == nil {
					return 0
				}
				return company.ID
			}(),
			"error": err.Error(),
		})
		cookieHeader = ""
	}

	companyID := 0
	if company != nil {
		companyID = company.ID
	}
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
	p.setCommonHeaders(req, endpoint, company, true)
	if strings.TrimSpace(cookieHeader) != "" {
		req.Header.Set("Cookie", cookieHeader)
	}

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
		careersURL := ""
		if company != nil {
			careersURL = company.CareersURL
		}
		return nil, 0, fmt.Errorf("workday returned status %d (endpoint=%s careers_url=%s): %s", resp.StatusCode, endpoint, careersURL, snippet)
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

func (p *WorkdayProvider) warmUpSession(ctx context.Context, endpoint string, company *models.JobCompany) (string, error) {
	if cached, ok := p.sessionCookies.Load(endpoint); ok {
		if header, valid := cached.(string); valid {
			return header, nil
		}
	}

	target := endpoint
	if company != nil && strings.TrimSpace(company.CareersURL) != "" {
		target = strings.TrimSpace(company.CareersURL)
		if parsed, err := url.Parse(target); err == nil && parsed.Host == "" {
			target = "https://" + target
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	p.setCommonHeaders(req, target, company, false)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if _, err := io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20)); err != nil {
		p.logger.Warn("error discarding response body during warmup", map[string]interface{}{"endpoint": endpoint, "error": err.Error()})
	}

	cookieHeader := ""
	if jar := p.client.Jar; jar != nil {
		if u, err := url.Parse(endpoint); err == nil {
			cookieHeader = buildCookieHeader(jar.Cookies(u))
		}
	}
	p.sessionCookies.Store(endpoint, cookieHeader)
	return cookieHeader, nil
}

func (p *WorkdayProvider) setCommonHeaders(req *http.Request, endpoint string, company *models.JobCompany, includeContentType bool) {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", defaultLocaleHeader)
	req.Header.Set("User-Agent", p.userAgent)
	req.Header.Set("X-Workday-Client", workdayClientHeader)
	if includeContentType {
		req.Header.Set("Content-Type", "application/json")
	}

	if parsed, err := url.Parse(endpoint); err == nil && parsed.Host != "" {
		origin := fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)
		req.Header.Set("Origin", origin)
		referer := origin
		if company != nil && strings.TrimSpace(company.CareersURL) != "" {
			referer = company.CareersURL
		}
		req.Header.Set("Referer", referer)
	}
}

func buildCookieHeader(cookies []*http.Cookie) string {
	if len(cookies) == 0 {
		return ""
	}
	parts := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie == nil || strings.TrimSpace(cookie.Name) == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", cookie.Name, cookie.Value))
	}
	return strings.Join(parts, "; ")
}
