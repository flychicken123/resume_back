package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"resumeai/config"
	"resumeai/utils"
)

// JobMatchingService coordinates calls to external job boards and scores matches against a resume profile.
type JobMatchingService struct {
	httpClient *http.Client
	cfg        config.JobBoardConfig
	logger     *utils.Logger
}

// ResumeData mirrors the structure used on the frontend resume builder so we can reuse rich content for matching.
type ResumeData struct {
	Name             string             `json:"name"`
	Email            string             `json:"email"`
	Phone            string             `json:"phone"`
	Summary          string             `json:"summary"`
	Skills           string             `json:"skills"`
	Experiences      []ResumeExperience `json:"experiences"`
	Education        []ResumeEducation  `json:"education"`
	Projects         []ResumeProject    `json:"projects"`
	SelectedFormat   string             `json:"selectedFormat"`
	SelectedFontSize string             `json:"selectedFontSize"`
	Address          string             `json:"address"`
	City             string             `json:"city"`
	State            string             `json:"state"`
	Country          string             `json:"country"`
}

// ResumeExperience represents a single work history entry.
type ResumeExperience struct {
	JobTitle         string `json:"jobTitle"`
	Company          string `json:"company"`
	City             string `json:"city"`
	State            string `json:"state"`
	Remote           bool   `json:"remote"`
	StartDate        string `json:"startDate"`
	EndDate          string `json:"endDate"`
	CurrentlyWorking bool   `json:"currentlyWorking"`
	Description      string `json:"description"`
}

// ResumeEducation is kept for potential future enrichment (e.g., matching degree requirements).
type ResumeEducation struct {
	Degree         string `json:"degree"`
	School         string `json:"school"`
	Field          string `json:"field"`
	GraduationYear string `json:"graduationYear"`
	GPA            string `json:"gpa"`
	Honors         string `json:"honors"`
	Location       string `json:"location"`
}

// ResumeProject provides supplemental context on skills and technologies.
type ResumeProject struct {
	ProjectName  string `json:"projectName"`
	Description  string `json:"description"`
	Technologies string `json:"technologies"`
	ProjectURL   string `json:"projectUrl"`
}

// JobMatch is the response payload returned to the frontend.
type JobMatch struct {
	Source          string    `json:"source"`
	SourceID        string    `json:"sourceId"`
	Title           string    `json:"title"`
	Company         string    `json:"company"`
	Location        string    `json:"location"`
	Remote          bool      `json:"remote"`
	ApplyURL        string    `json:"applyUrl"`
	Description     string    `json:"description"`
	JobType         string    `json:"jobType,omitempty"`
	Category        string    `json:"category,omitempty"`
	PublishedAt     time.Time `json:"publishedAt"`
	Score           float64   `json:"score"`
	ScoreLabel      string    `json:"scoreLabel"`
	MatchedSkills   []string  `json:"matchedSkills,omitempty"`
	MatchedKeywords []string  `json:"matchedKeywords,omitempty"`
}

// JobMatchResult wraps aggregated matches and additional metadata useful to the UI.
type JobMatchResult struct {
	Jobs             []JobMatch `json:"jobs"`
	ResumeKeywords   []string   `json:"resumeKeywords"`
	ProvidersUsed    []string   `json:"providersUsed"`
	ProvidersSkipped []string   `json:"providersSkipped,omitempty"`
	CountryHint      string     `json:"countryHint,omitempty"`
}

// jobPosting represents a normalized record we can score irrespective of data source.
type jobPosting struct {
	Source         string
	SourceKey      string
	Title          string
	Company        string
	Location       string
	Remote         bool
	ApplyURL       string
	Description    string
	JobType        string
	Category       string
	PublishedAt    time.Time
	Tags           []string
	NormalizedText string
}

// resumeProfile holds precomputed keywords and metadata extracted from the resume.
type resumeProfile struct {
	TargetTitle      string
	SkillPhrases     []string
	SkillTokens      []string
	KeywordTokens    []string
	QueryTerms       []string
	Locations        []string
	PrimaryLocation  string
	ResumeKeywords   []string
	CountryHint      string
	RemotePreference bool
}

var htmlTagRegex = regexp.MustCompile(`(?s)<[^>]*>`)
var nonAlphaNumericRegex = regexp.MustCompile(`[^a-z0-9]+`)

var diacriticTransformer = transform.Chain(
	norm.NFD,
	transform.RemoveFunc(func(r rune) bool {
		return unicode.Is(unicode.Mn, r)
	}),
	norm.NFC,
)

var specialRuneReplacer = strings.NewReplacer("ß", "ss", "ẞ", "ss")

var regionToCountry = map[string]string{
	"AL": "us", "AK": "us", "AZ": "us", "AR": "us", "CA": "us", "CO": "us", "CT": "us", "DE": "us",
	"FL": "us", "GA": "us", "HI": "us", "ID": "us", "IL": "us", "IN": "us", "IA": "us", "KS": "us",
	"KY": "us", "LA": "us", "ME": "us", "MD": "us", "MA": "us", "MI": "us", "MN": "us", "MS": "us",
	"MO": "us", "MT": "us", "NE": "us", "NV": "us", "NH": "us", "NJ": "us", "NM": "us", "NY": "us",
	"NC": "us", "ND": "us", "OH": "us", "OK": "us", "OR": "us", "PA": "us", "RI": "us", "SC": "us",
	"SD": "us", "TN": "us", "TX": "us", "UT": "us", "VT": "us", "VA": "us", "WA": "us", "WI": "us",
	"WV": "us", "WY": "us", "DC": "us", "PR": "us",
	"ON": "ca", "QC": "ca", "BC": "ca", "AB": "ca", "MB": "ca", "NB": "ca", "NS": "ca", "PE": "ca", "SK": "ca", "NL": "ca",
	"ACT": "au", "NSW": "au", "NT": "au", "QLD": "au", "SA": "au", "TAS": "au", "VIC": "au",
	"LDN": "uk", "ENG": "uk", "SCT": "uk", "WLS": "uk", "NIR": "uk",
}

var countryAliases = map[string][]string{
	"us": {"united states", "usa", "u.s.", "united states of america", "america"},
	"uk": {"united kingdom", "uk", "england", "great britain", "britain", "scotland", "wales", "northern ireland"},
	"ca": {"canada"},
	"au": {"australia"},
	"de": {"germany", "deutschland"},
	"fr": {"france"},
	"in": {"india"},
	"ie": {"ireland"},
	"sg": {"singapore"},
	"nz": {"new zealand"},
	"es": {"spain", "españa"},
	"cn": {"china", "prc", "中华人民共和国"},
	"it": {"italy", "italia"},
	"nl": {"netherlands", "holland"},
	"se": {"sweden"},
	"no": {"norway"},
	"fi": {"finland"},
}

var cityToCountry = map[string]string{
	"berlin":        "de",
	"hamburg":       "de",
	"munich":        "de",
	"munchen":       "de",
	"muenchen":      "de",
	"stuttgart":     "de",
	"frankfurt":     "de",
	"cologne":       "de",
	"koln":          "de",
	"koeln":         "de",
	"dusseldorf":    "de",
	"duesseldorf":   "de",
	"dortmund":      "de",
	"essen":         "de",
	"leipzig":       "de",
	"hannover":      "de",
	"nuremberg":     "de",
	"nuernberg":     "de",
	"nurnberg":      "de",
	"augsburg":      "de",
	"bremen":        "de",
	"bonn":          "de",
	"dresden":       "de",
	"freiburg":      "de",
	"heidelberg":    "de",
	"kassel":        "de",
	"karlsruhe":     "de",
	"mannheim":      "de",
	"saarbrucken":   "de",
	"saarbruecken":  "de",
	"wiesbaden":     "de",
	"aachen":        "de",
	"ulm":           "de",
	"jena":          "de",
	"rostock":       "de",
	"kiel":          "de",
	"magdeburg":     "de",
	"bochum":        "de",
	"braunschweig":  "de",
	"chemnitz":      "de",
	"duisburg":      "de",
	"gelsenkirchen": "de",
	"zurich":        "ch",
	"geneva":        "ch",
	"bern":          "ch",
	"basel":         "ch",
	"lausanne":      "ch",
	"lucerne":       "ch",
	"vienna":        "at",
	"wien":          "at",
	"salzburg":      "at",
	"graz":          "at",
	"innsbruck":     "at",
	"linz":          "at",
	"paris":         "fr",
	"lyon":          "fr",
	"marseille":     "fr",
	"toulouse":      "fr",
	"madrid":        "es",
	"barcelona":     "es",
	"valencia":      "es",
	"lisbon":        "pt",
	"porto":         "pt",
	"london":        "uk",
	"manchester":    "uk",
	"birmingham":    "uk",
	"edinburgh":     "uk",
	"dublin":        "ie",
	"toronto":       "ca",
	"vancouver":     "ca",
	"montreal":      "ca",
	"ottawa":        "ca",
	"calgary":       "ca",
	"sydney":        "au",
	"melbourne":     "au",
	"brisbane":      "au",
	"perth":         "au",
	"singapore":     "sg",
	"bangalore":     "in",
	"bengaluru":     "in",
	"mumbai":        "in",
	"hyderabad":     "in",
	"delhi":         "in",
	"new delhi":     "in",
	"gurgaon":       "in",
	"gurugram":      "in",
	"pune":          "in",
	"san francisco": "us",
	"new york":      "us",
	"seattle":       "us",
	"austin":        "us",
	"dallas":        "us",
	"chicago":       "us",
	"atlanta":       "us",
	"denver":        "us",
	"boston":        "us",
	"miami":         "us",
	"los angeles":   "us",
	"washington":    "us",
}

const (
	defaultJobMatchLimit = 15
	maxKeywordCount      = 25
)

// NewJobMatchingService builds a new service using the configured HTTP timeout.
func NewJobMatchingService(cfg config.JobBoardConfig, logger *utils.Logger) *JobMatchingService {
	client := &http.Client{Timeout: cfg.RequestTimeout}
	if logger == nil {
		logger = utils.NewLogger()
	}
	return &JobMatchingService{
		httpClient: client,
		cfg:        cfg,
		logger:     logger,
	}
}

// FindMatches aggregates job postings from configured providers and ranks them against the resume.
func (s *JobMatchingService) FindMatches(ctx context.Context, resume ResumeData, jobDescription string, limit int, clientCountry string) (JobMatchResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	profile := buildResumeProfile(resume, jobDescription)

	if profile.CountryHint == "" && strings.TrimSpace(clientCountry) != "" {
		profile.CountryHint = normalizeCountryCode(clientCountry)
	}

	if limit <= 0 {
		limit = defaultJobMatchLimit
	}
	if limit > 50 {
		limit = 50
	}

	postings := make([]jobPosting, 0, 64)
	providersUsed := make([]string, 0)
	providersSkipped := make([]string, 0)

	for _, token := range s.cfg.GreenhouseBoardTokens {
		jobs, err := s.fetchGreenhouseJobs(ctx, token)
		if err != nil {
			providersSkipped = append(providersSkipped, fmt.Sprintf("greenhouse:%s", token))
			s.logger.Warn("greenhouse_fetch_failed", map[string]interface{}{"token": token, "error": err.Error()})
			continue
		}
		if len(jobs) > 0 {
			postings = append(postings, jobs...)
			providersUsed = append(providersUsed, fmt.Sprintf("greenhouse:%s", token))
		}
	}

	for _, slug := range s.cfg.LeverCompanySlugs {
		jobs, err := s.fetchLeverJobs(ctx, slug)
		if err != nil {
			providersSkipped = append(providersSkipped, fmt.Sprintf("lever:%s", slug))
			s.logger.Warn("lever_fetch_failed", map[string]interface{}{"slug": slug, "error": err.Error()})
			continue
		}
		if len(jobs) > 0 {
			postings = append(postings, jobs...)
			providersUsed = append(providersUsed, fmt.Sprintf("lever:%s", slug))
		}
	}

	if s.cfg.RemotiveEnabled {
		jobs, err := s.fetchRemotiveJobs(ctx, profile)
		if err != nil {
			providersSkipped = append(providersSkipped, "remotive")
			s.logger.Warn("remotive_fetch_failed", map[string]interface{}{"error": err.Error()})
		} else if len(jobs) > 0 {
			postings = append(postings, jobs...)
			providersUsed = append(providersUsed, "remotive")
		}
	}

	if s.cfg.ArbeitnowEnabled {
		jobs, err := s.fetchArbeitnowJobs(ctx, profile)
		if err != nil {
			providersSkipped = append(providersSkipped, "arbeitnow")
			s.logger.Warn("arbeitnow_fetch_failed", map[string]interface{}{"error": err.Error()})
		} else if len(jobs) > 0 {
			postings = append(postings, jobs...)
			providersUsed = append(providersUsed, "arbeitnow")
		}
	}

	if s.cfg.SmartRecruitersEnabled {
		for _, slug := range s.cfg.SmartRecruitersCompanySlugs {
			trimmed := strings.TrimSpace(slug)
			if trimmed == "" {
				continue
			}

			jobs, err := s.fetchSmartRecruitersJobs(ctx, trimmed, profile)
			if err != nil {
				providersSkipped = append(providersSkipped, fmt.Sprintf("smartrecruiters:%s", trimmed))
				s.logger.Warn("smartrecruiters_fetch_failed", map[string]interface{}{"slug": trimmed, "error": err.Error()})
				continue
			}

			if len(jobs) > 0 {
				postings = append(postings, jobs...)
				providersUsed = append(providersUsed, fmt.Sprintf("smartrecruiters:%s", trimmed))
			}
		}
	}

	if s.cfg.MuseEnabled {
		jobs, err := s.fetchMuseJobs(ctx, profile)
		if err != nil {
			providersSkipped = append(providersSkipped, "muse")
			s.logger.Warn("muse_fetch_failed", map[string]interface{}{"error": err.Error()})
		} else if len(jobs) > 0 {
			postings = append(postings, jobs...)
			providersUsed = append(providersUsed, "muse")
		}
	}

	if s.cfg.WellfoundEnabled {
		jobs, err := s.fetchWellfoundJobs(ctx, profile)
		if err != nil {
			providersSkipped = append(providersSkipped, "wellfound")
			s.logger.Warn("wellfound_fetch_failed", map[string]interface{}{"error": err.Error()})
		} else if len(jobs) > 0 {
			postings = append(postings, jobs...)
			providersUsed = append(providersUsed, "wellfound")
		}
	}

	if s.cfg.USAJobsEnabled {
		jobs, err := s.fetchUSAJobs(ctx, profile)
		if err != nil {
			providersSkipped = append(providersSkipped, "usajobs")
			s.logger.Warn("usajobs_fetch_failed", map[string]interface{}{"error": err.Error()})
		} else if len(jobs) > 0 {
			postings = append(postings, jobs...)
			providersUsed = append(providersUsed, "usajobs")
		}
	}

	if s.cfg.SimplifyEnabled {
		jobs, err := s.fetchSimplifyJobs(ctx, profile)
		if err != nil {
			providersSkipped = append(providersSkipped, "simplify")
			s.logger.Warn("simplify_fetch_failed", map[string]interface{}{"error": err.Error()})
		} else if len(jobs) > 0 {
			postings = append(postings, jobs...)
			providersUsed = append(providersUsed, "simplify")
		}
	}

	postings = deduplicatePostings(postings)
	postings = filterPostingsByCountry(profile.CountryHint, postings)
	matches := rankJobMatches(profile, postings, limit)

	return JobMatchResult{
		Jobs:             matches,
		ResumeKeywords:   profile.ResumeKeywords,
		ProvidersUsed:    providersUsed,
		ProvidersSkipped: providersSkipped,
		CountryHint:      profile.CountryHint,
	}, nil
}
func (s *JobMatchingService) fetchGreenhouseJobs(ctx context.Context, boardToken string) ([]jobPosting, error) {
	trimmed := strings.TrimSpace(boardToken)
	if trimmed == "" {
		return nil, fmt.Errorf("empty board token")
	}

	base := strings.TrimSuffix(s.cfg.GreenhouseBaseURL, "/")
	endpoint := fmt.Sprintf("%s/v1/boards/%s/jobs?content=true", base, url.PathEscape(trimmed))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("greenhouse request failed with status %d", resp.StatusCode)
	}

	var payload struct {
		Jobs []struct {
			ID        int    `json:"id"`
			Title     string `json:"title"`
			UpdatedAt string `json:"updated_at"`
			Location  struct {
				Name string `json:"name"`
			} `json:"location"`
			AbsoluteURL string `json:"absolute_url"`
			Content     string `json:"content"`
			Metadata    []struct {
				Name  string      `json:"name"`
				Value interface{} `json:"value"`
			} `json:"metadata"`
			Department struct {
				Name string `json:"name"`
			} `json:"department"`
		} `json:"jobs"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	postings := make([]jobPosting, 0, len(payload.Jobs))
	companyName := humanizeIdentifier(trimmed)

	for _, job := range payload.Jobs {
		publishedAt := time.Now()
		if job.UpdatedAt != "" {
			if t, parseErr := time.Parse(time.RFC3339, job.UpdatedAt); parseErr == nil {
				publishedAt = t
			}
		}

		desc := sanitizeDescription(job.Content)
		jobType, tags := parseGreenhouseMetadata(job.Metadata)

		normalizedParts := []string{
			strings.ToLower(job.Title),
			strings.ToLower(desc),
			strings.ToLower(job.Location.Name),
		}
		if companyName != "" {
			normalizedParts = append(normalizedParts, strings.ToLower(companyName))
		}
		if job.Department.Name != "" {
			normalizedParts = append(normalizedParts, strings.ToLower(job.Department.Name))
		}
		if jobType != "" {
			normalizedParts = append(normalizedParts, strings.ToLower(jobType))
		}
		if len(tags) > 0 {
			normalizedParts = append(normalizedParts, strings.ToLower(strings.Join(tags, " ")))
		}

		postings = append(postings, jobPosting{
			Source:         "greenhouse",
			SourceKey:      fmt.Sprintf("%s:%d", trimmed, job.ID),
			Title:          job.Title,
			Company:        companyName,
			Location:       job.Location.Name,
			Remote:         strings.Contains(strings.ToLower(job.Location.Name), "remote"),
			ApplyURL:       job.AbsoluteURL,
			Description:    desc,
			JobType:        jobType,
			Category:       job.Department.Name,
			PublishedAt:    publishedAt,
			Tags:           tags,
			NormalizedText: strings.Join(normalizedParts, " "),
		})
	}

	if s.cfg.MaxJobsPerProvider > 0 && len(postings) > s.cfg.MaxJobsPerProvider {
		postings = postings[:s.cfg.MaxJobsPerProvider]
	}

	return postings, nil
}

func parseGreenhouseMetadata(metadata []struct {
	Name  string      `json:"name"`
	Value interface{} `json:"value"`
}) (string, []string) {
	jobType := ""
	tags := make([]string, 0)
	for _, item := range metadata {
		value := fmt.Sprintf("%v", item.Value)
		switch strings.ToLower(item.Name) {
		case "employment type", "job type", "contract type":
			jobType = value
		default:
			if value != "" {
				tags = append(tags, value)
			}
		}
	}
	return jobType, tags
}
func (s *JobMatchingService) fetchLeverJobs(ctx context.Context, slug string) ([]jobPosting, error) {
	trimmed := strings.TrimSpace(slug)
	if trimmed == "" {
		return nil, fmt.Errorf("empty lever slug")
	}

	base := strings.TrimSuffix(s.cfg.LeverBaseURL, "/")
	endpoint := fmt.Sprintf("%s/v0/postings/%s?mode=json", base, url.PathEscape(trimmed))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return []jobPosting{}, nil
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("lever request failed with status %d", resp.StatusCode)
	}

	var payload []struct {
		ID          string `json:"id"`
		Text        string `json:"text"`
		HostedURL   string `json:"hostedUrl"`
		CreatedAt   int64  `json:"createdAt"`
		UpdatedAt   int64  `json:"updatedAt"`
		Description string `json:"description"`
		Categories  struct {
			Team       string `json:"team"`
			Commitment string `json:"commitment"`
			Location   string `json:"location"`
		} `json:"categories"`
		Tags []string `json:"tags"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	postings := make([]jobPosting, 0, len(payload))
	companyName := humanizeIdentifier(trimmed)

	for _, post := range payload {
		publishedAt := time.Now()
		if post.CreatedAt > 0 {
			publishedAt = time.UnixMilli(post.CreatedAt)
		} else if post.UpdatedAt > 0 {
			publishedAt = time.UnixMilli(post.UpdatedAt)
		}

		desc := sanitizeDescription(post.Description)
		normalizedParts := []string{
			strings.ToLower(post.Text),
			strings.ToLower(desc),
			strings.ToLower(post.Categories.Location),
		}
		if post.Categories.Team != "" {
			normalizedParts = append(normalizedParts, strings.ToLower(post.Categories.Team))
		}
		if post.Categories.Commitment != "" {
			normalizedParts = append(normalizedParts, strings.ToLower(post.Categories.Commitment))
		}
		if len(post.Tags) > 0 {
			normalizedParts = append(normalizedParts, strings.ToLower(strings.Join(post.Tags, " ")))
		}

		postings = append(postings, jobPosting{
			Source:         "lever",
			SourceKey:      fmt.Sprintf("%s:%s", trimmed, post.ID),
			Title:          post.Text,
			Company:        companyName,
			Location:       post.Categories.Location,
			Remote:         strings.Contains(strings.ToLower(post.Categories.Location), "remote"),
			ApplyURL:       post.HostedURL,
			Description:    desc,
			JobType:        post.Categories.Commitment,
			Category:       post.Categories.Team,
			PublishedAt:    publishedAt,
			Tags:           post.Tags,
			NormalizedText: strings.Join(normalizedParts, " "),
		})
	}

	if s.cfg.MaxJobsPerProvider > 0 && len(postings) > s.cfg.MaxJobsPerProvider {
		postings = postings[:s.cfg.MaxJobsPerProvider]
	}

	return postings, nil
}

func (s *JobMatchingService) fetchRemotiveJobs(ctx context.Context, profile resumeProfile) ([]jobPosting, error) {
	base := strings.TrimSuffix(s.cfg.RemotiveBaseURL, "/")
	limit := s.cfg.MaxJobsPerProvider
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	endpoint := fmt.Sprintf("%s/api/remote-jobs?limit=%d", base, limit)
	if len(profile.QueryTerms) > 0 {
		query := strings.Join(profile.QueryTerms, " ")
		endpoint = endpoint + "&search=" + url.QueryEscape(query)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("remotive request failed with status %d", resp.StatusCode)
	}

	var payload struct {
		Jobs []struct {
			ID                        int      `json:"id"`
			Title                     string   `json:"title"`
			CompanyName               string   `json:"company_name"`
			URL                       string   `json:"url"`
			Description               string   `json:"description"`
			JobType                   string   `json:"job_type"`
			Category                  string   `json:"category"`
			CandidateRequiredLocation string   `json:"candidate_required_location"`
			Tags                      []string `json:"tags"`
			PublicationDate           string   `json:"publication_date"`
		} `json:"jobs"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	postings := make([]jobPosting, 0, len(payload.Jobs))

	for _, job := range payload.Jobs {
		publishedAt := time.Now()
		if job.PublicationDate != "" {
			if t, parseErr := time.Parse(time.RFC3339, job.PublicationDate); parseErr == nil {
				publishedAt = t
			}
		}

		desc := sanitizeDescription(job.Description)
		normalizedParts := []string{
			strings.ToLower(job.Title),
			strings.ToLower(desc),
			strings.ToLower(job.CandidateRequiredLocation),
		}
		if job.Category != "" {
			normalizedParts = append(normalizedParts, strings.ToLower(job.Category))
		}
		if job.JobType != "" {
			normalizedParts = append(normalizedParts, strings.ToLower(job.JobType))
		}
		if len(job.Tags) > 0 {
			normalizedParts = append(normalizedParts, strings.ToLower(strings.Join(job.Tags, " ")))
		}

		company := strings.TrimSpace(job.CompanyName)
		postings = append(postings, jobPosting{
			Source:         "remotive",
			SourceKey:      fmt.Sprintf("remotive:%d", job.ID),
			Title:          job.Title,
			Company:        company,
			Location:       job.CandidateRequiredLocation,
			Remote:         true,
			ApplyURL:       job.URL,
			Description:    desc,
			JobType:        job.JobType,
			Category:       job.Category,
			PublishedAt:    publishedAt,
			Tags:           job.Tags,
			NormalizedText: strings.Join(normalizedParts, " "),
		})
	}

	return postings, nil
}

func (s *JobMatchingService) fetchArbeitnowJobs(ctx context.Context, profile resumeProfile) ([]jobPosting, error) {
	base := strings.TrimSuffix(s.cfg.ArbeitnowBaseURL, "/")
	limit := s.cfg.MaxJobsPerProvider
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	endpoint := fmt.Sprintf("%s/api/job-board-api?page=1&size=%d", base, limit)
	if len(profile.QueryTerms) > 0 {
		endpoint = endpoint + "&search=" + url.QueryEscape(strings.Join(profile.QueryTerms, " "))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("arbeitnow request failed with status %d", resp.StatusCode)
	}

	var payload struct {
		Data []struct {
			Slug        string   `json:"slug"`
			CompanyName string   `json:"company_name"`
			Title       string   `json:"title"`
			Description string   `json:"description"`
			Remote      bool     `json:"remote"`
			URL         string   `json:"url"`
			Tags        []string `json:"tags"`
			JobTypes    []string `json:"job_types"`
			Location    string   `json:"location"`
			CreatedAt   int64    `json:"created_at"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	postings := make([]jobPosting, 0, len(payload.Data))

	for _, job := range payload.Data {
		publishedAt := time.Now()
		if job.CreatedAt > 0 {
			publishedAt = time.Unix(job.CreatedAt, 0)
		}

		desc := sanitizeDescription(job.Description)
		normalizedParts := []string{
			strings.ToLower(job.Title),
			strings.ToLower(desc),
			strings.ToLower(job.Location),
		}
		if len(job.JobTypes) > 0 {
			normalizedParts = append(normalizedParts, strings.ToLower(strings.Join(job.JobTypes, " ")))
		}
		if len(job.Tags) > 0 {
			normalizedParts = append(normalizedParts, strings.ToLower(strings.Join(job.Tags, " ")))
		}

		posting := jobPosting{
			Source:         "arbeitnow",
			SourceKey:      fmt.Sprintf("arbeitnow:%s", job.Slug),
			Title:          job.Title,
			Company:        job.CompanyName,
			Location:       job.Location,
			Remote:         job.Remote || strings.Contains(strings.ToLower(job.Location), "remote"),
			ApplyURL:       job.URL,
			Description:    desc,
			JobType:        strings.Join(job.JobTypes, ", "),
			Category:       "",
			PublishedAt:    publishedAt,
			Tags:           job.Tags,
			NormalizedText: strings.Join(normalizedParts, " "),
		}

		if profile.CountryHint != "" {
			if !matchesCountryHint(posting, profile.CountryHint) {
				if !(posting.Remote && isRemoteWorldwide(posting)) {
					continue
				}
			}
		}

		postings = append(postings, posting)
	}

	return postings, nil
}

func (s *JobMatchingService) fetchSmartRecruitersJobs(ctx context.Context, slug string, profile resumeProfile) ([]jobPosting, error) {
	base := strings.TrimSpace(s.cfg.SmartRecruitersBaseURL)
	if base == "" {
		base = "https://api.smartrecruiters.com"
	}
	base = strings.TrimSuffix(base, "/")

	limit := s.cfg.MaxJobsPerProvider
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	postings := make([]jobPosting, 0, limit)
	offset := 0

	fetchDetail := func(jobID string) (map[string]interface{}, error) {
		detailURL := fmt.Sprintf("%s/v1/companies/%s/postings/%s", base, url.PathEscape(slug), url.PathEscape(jobID))
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, detailURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "resumeai-job-matcher/1.0")

		resp, err := s.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			return nil, fmt.Errorf("smartrecruiters detail failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
		}

		var detail map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
			return nil, err
		}
		return detail, nil
	}

	for len(postings) < limit {
		remaining := limit - len(postings)
		batchLimit := remaining
		if batchLimit > 50 {
			batchLimit = 50
		}

		listURL := fmt.Sprintf("%s/v1/companies/%s/postings", base, url.PathEscape(slug))
		u, err := url.Parse(listURL)
		if err != nil {
			return nil, err
		}
		query := u.Query()
		query.Set("offset", strconv.Itoa(offset))
		query.Set("limit", strconv.Itoa(batchLimit))
		u.RawQuery = query.Encode()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "resumeai-job-matcher/1.0")

		resp, err := s.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= 400 {
			snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			resp.Body.Close()
			return nil, fmt.Errorf("smartrecruiters request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
		}

		var payload struct {
			Content []struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				Ref      string `json:"ref"`
				ApplyURL string `json:"applyUrl"`
				Released string `json:"releasedDate"`
				Company  struct {
					Name string `json:"name"`
				} `json:"company"`
				Location struct {
					Address     string `json:"address"`
					City        string `json:"city"`
					Region      string `json:"region"`
					Country     string `json:"country"`
					CountryCode string `json:"countryCode"`
					Remote      bool   `json:"remote"`
				} `json:"location"`
				JobAd struct {
					Sections map[string]struct {
						Title string `json:"title"`
						Text  string `json:"text"`
					} `json:"sections"`
				} `json:"jobAd"`
			} `json:"content"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()

		if len(payload.Content) == 0 {
			break
		}

		for _, job := range payload.Content {
			title := strings.TrimSpace(job.Name)
			if title == "" {
				continue
			}

			desc := ""
			if job.JobAd.Sections != nil {
				descBuilder := strings.Builder{}
				priority := []string{"JOB_DESCRIPTION", "JOB_DESCRIPTION_SECTION", "QUALIFICATIONS", "COMPANY_DESCRIPTION"}
				seen := make(map[string]struct{})
				for _, key := range priority {
					if section, ok := job.JobAd.Sections[key]; ok {
						if text := strings.TrimSpace(section.Text); text != "" {
							descBuilder.WriteString(text)
							descBuilder.WriteString("\n\n")
							seen[key] = struct{}{}
						}
					}
				}
				for key, section := range job.JobAd.Sections {
					if _, exists := seen[key]; exists {
						continue
					}
					if text := strings.TrimSpace(section.Text); text != "" {
						descBuilder.WriteString(text)
						descBuilder.WriteString("\n\n")
					}
				}
				desc = descBuilder.String()
			}

			if strings.TrimSpace(desc) == "" {
				detail, err := fetchDetail(job.ID)
				if err == nil {
					if jobAd, ok := extractNested(detail, "jobAd").(map[string]interface{}); ok {
						if sections, ok := jobAd["sections"].(map[string]interface{}); ok {
							descBuilder := strings.Builder{}
							for _, key := range []string{"JOB_DESCRIPTION", "JOB_DESCRIPTION_SECTION", "QUALIFICATIONS", "COMPANY_DESCRIPTION"} {
								if section, ok := sections[key].(map[string]interface{}); ok {
									if text := strings.TrimSpace(coerceString(section["text"])); text != "" {
										descBuilder.WriteString(text)
										descBuilder.WriteString("\n\n")
									}
								}
							}
							desc = descBuilder.String()
						}
					}
				}
			}

			desc = sanitizeDescription(desc)

			locationParts := make([]string, 0, 4)
			if trimmed := strings.TrimSpace(job.Location.Address); trimmed != "" {
				locationParts = append(locationParts, trimmed)
			}
			if trimmed := strings.TrimSpace(job.Location.City); trimmed != "" {
				locationParts = append(locationParts, trimmed)
			}
			if trimmed := strings.TrimSpace(job.Location.Region); trimmed != "" {
				locationParts = append(locationParts, trimmed)
			}
			if trimmed := strings.TrimSpace(job.Location.Country); trimmed != "" {
				locationParts = append(locationParts, trimmed)
			}
			location := strings.Join(uniqueStrings(locationParts), ", ")
			if location == "" {
				location = strings.TrimSpace(job.Location.Country)
			}

			applyURL := strings.TrimSpace(job.ApplyURL)
			if applyURL == "" && job.Ref != "" {
				applyURL = fmt.Sprintf("https://www.smartrecruiters.com/%s/%s", slug, job.Ref)
			}

			sourceKey := strings.TrimSpace(job.ID)
			if sourceKey == "" {
				sourceKey = strings.TrimSpace(job.Ref)
			}
			if sourceKey == "" && applyURL != "" {
				sourceKey = applyURL
			}
			if sourceKey == "" {
				sourceKey = strings.ToLower(strings.ReplaceAll(title, " ", "-"))
			}

			publishedAt := time.Now()
			if job.Released != "" {
				if t, err := time.Parse(time.RFC3339, job.Released); err == nil {
					publishedAt = t
				}
			}

			normalizedParts := []string{
				strings.ToLower(title),
				strings.ToLower(desc),
				strings.ToLower(location),
			}
			if company := strings.TrimSpace(job.Company.Name); company != "" {
				normalizedParts = append(normalizedParts, strings.ToLower(company))
			}

			posting := jobPosting{
				Source:         "smartrecruiters",
				SourceKey:      fmt.Sprintf("smartrecruiters:%s:%s", slug, sourceKey),
				Title:          title,
				Company:        strings.TrimSpace(job.Company.Name),
				Location:       location,
				Remote:         job.Location.Remote || strings.Contains(strings.ToLower(desc), "remote"),
				ApplyURL:       applyURL,
				Description:    desc,
				PublishedAt:    publishedAt,
				NormalizedText: strings.Join(normalizedParts, " "),
			}

			if profile.CountryHint != "" && !matchesCountryHint(posting, profile.CountryHint) {
				if !(posting.Remote && isRemoteWorldwide(posting)) {
					continue
				}
			}

			postings = append(postings, posting)
			if len(postings) >= limit {
				break
			}
		}

		offset += len(payload.Content)
		if len(payload.Content) < batchLimit {
			break
		}
	}

	return postings, nil
}

func (s *JobMatchingService) fetchMuseJobs(ctx context.Context, profile resumeProfile) ([]jobPosting, error) {
	base := strings.TrimSpace(s.cfg.MuseBaseURL)
	if base == "" {
		return nil, fmt.Errorf("muse base url not configured")
	}
	base = strings.TrimSuffix(base, "/")

	limit := s.cfg.MaxJobsPerProvider
	if limit <= 0 || limit > 50 {
		limit = 50
	}

	params := url.Values{}
	params.Set("page", "1")
	params.Set("descending", "true")
	params.Set("items_per_page", strconv.Itoa(limit))

	locationQuery := strings.TrimSpace(profile.PrimaryLocation)
	if locationQuery == "" && profile.CountryHint != "" {
		if strings.EqualFold(profile.CountryHint, "us") {
			locationQuery = "United States"
		} else {
			locationQuery = strings.ToUpper(profile.CountryHint)
		}
	}
	if locationQuery != "" {
		params.Set("location", locationQuery)
	}

	if len(profile.QueryTerms) > 0 {
		params.Set("category", strings.Join(profile.QueryTerms[:min(3, len(profile.QueryTerms))], ","))
	}

	endpoint := fmt.Sprintf("%s/api/public/jobs?%s", base, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if key := strings.TrimSpace(s.cfg.MuseAPIKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("muse request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	var payload struct {
		Results []struct {
			Name    string `json:"name"`
			Company struct {
				Name string `json:"name"`
			} `json:"company"`
			Locations []struct {
				Name string `json:"name"`
			} `json:"locations"`
			PublicationDate string `json:"publication_date"`
			Refs            struct {
				LandingPage string `json:"landing_page"`
			} `json:"refs"`
			Contents string `json:"contents"`
			Levels   []struct {
				Name string `json:"name"`
			} `json:"levels"`
			Categories []struct {
				Name string `json:"name"`
			} `json:"categories"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	postings := make([]jobPosting, 0, len(payload.Results))
	for _, job := range payload.Results {
		title := strings.TrimSpace(job.Name)
		if title == "" {
			continue
		}

		locationParts := make([]string, 0, len(job.Locations))
		for _, loc := range job.Locations {
			if trimmed := strings.TrimSpace(loc.Name); trimmed != "" {
				locationParts = append(locationParts, trimmed)
			}
		}
		location := strings.Join(locationParts, ", ")

		desc := sanitizeDescription(job.Contents)
		jobType := ""
		if len(job.Levels) > 0 {
			jobType = strings.TrimSpace(job.Levels[0].Name)
		}
		category := ""
		if len(job.Categories) > 0 {
			category = strings.TrimSpace(job.Categories[0].Name)
		}

		applyURL := strings.TrimSpace(job.Refs.LandingPage)
		sourceKey := applyURL
		if sourceKey == "" {
			sourceKey = strings.ToLower(strings.ReplaceAll(title, " ", "-"))
		}

		remote := strings.Contains(strings.ToLower(location), "remote") || strings.Contains(strings.ToLower(desc), "remote work")

		publishedAt := time.Now()
		if job.PublicationDate != "" {
			if t, err := time.Parse(time.RFC3339, job.PublicationDate); err == nil {
				publishedAt = t
			} else if len(job.PublicationDate) >= 10 {
				if t, err := time.Parse("2006-01-02", job.PublicationDate[:10]); err == nil {
					publishedAt = t
				}
			}
		}

		normalizedParts := []string{
			strings.ToLower(title),
			strings.ToLower(desc),
			strings.ToLower(location),
		}
		if jobType != "" {
			normalizedParts = append(normalizedParts, strings.ToLower(jobType))
		}
		if category != "" {
			normalizedParts = append(normalizedParts, strings.ToLower(category))
		}

		postings = append(postings, jobPosting{
			Source:         "muse",
			SourceKey:      fmt.Sprintf("muse:%s", sourceKey),
			Title:          title,
			Company:        strings.TrimSpace(job.Company.Name),
			Location:       location,
			Remote:         remote,
			ApplyURL:       applyURL,
			Description:    desc,
			JobType:        jobType,
			Category:       category,
			PublishedAt:    publishedAt,
			Tags:           nil,
			NormalizedText: strings.Join(normalizedParts, " "),
		})
	}

	if len(postings) > limit {
		postings = postings[:limit]
	}

	return postings, nil
}

func (s *JobMatchingService) fetchWellfoundJobs(ctx context.Context, profile resumeProfile) ([]jobPosting, error) {
	base := strings.TrimSpace(s.cfg.WellfoundBaseURL)
	token := strings.TrimSpace(s.cfg.WellfoundAPIToken)
	if base == "" || token == "" {
		return nil, fmt.Errorf("wellfound credentials not configured")
	}

	limit := s.cfg.MaxJobsPerProvider
	if limit <= 0 || limit > 50 {
		limit = 50
	}

	filter := map[string]interface{}{}
	if len(profile.QueryTerms) > 0 {
		filter["keyword"] = strings.Join(profile.QueryTerms[:min(5, len(profile.QueryTerms))], " ")
	}

	locations := make([]string, 0, 2)
	if profile.PrimaryLocation != "" {
		locations = append(locations, profile.PrimaryLocation)
	}
	if profile.CountryHint != "" {
		locations = append(locations, strings.ToUpper(profile.CountryHint))
	}
	if len(locations) > 0 {
		filter["locationNames"] = locations
	}
	if profile.RemotePreference {
		filter["remote"] = true
	}

	variables := map[string]interface{}{"page": 1}
	if len(filter) > 0 {
		variables["filter"] = filter
	}

	query := `query Jobs($page: Int!, $filter: JobsFilter) {
  jobs(page: $page, filter: $filter) {
    edges {
      node {
        id
        title
        description
        applyUrl
        publishedAt
        remote
        company { name }
        locations { name }
        tags { name }
      }
    }
  }
}`

	body := map[string]interface{}{
		"query":     query,
		"variables": variables,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("wellfound request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	var root map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&root); err != nil {
		return nil, err
	}

	if errsRaw, ok := root["errors"]; ok {
		if errs, ok := errsRaw.([]interface{}); ok && len(errs) > 0 {
			return nil, fmt.Errorf("wellfound returned an error response")
		}
	}

	data, _ := root["data"].(map[string]interface{})
	jobsContainer, _ := data["jobs"].(map[string]interface{})
	edgesRaw, _ := jobsContainer["edges"].([]interface{})

	postings := make([]jobPosting, 0, len(edgesRaw))
	for _, edge := range edgesRaw {
		edgeMap, _ := edge.(map[string]interface{})
		node, _ := edgeMap["node"].(map[string]interface{})
		if node == nil {
			continue
		}

		title := strings.TrimSpace(coerceString(node["title"]))
		if title == "" {
			continue
		}

		description := sanitizeDescription(coerceString(node["description"]))
		companyName := strings.TrimSpace(coerceString(extractNested(node, "company", "name")))
		applyURL := strings.TrimSpace(coerceString(node["applyUrl"]))
		if applyURL == "" {
			applyURL = strings.TrimSpace(coerceString(node["apply_url"]))
		}

		locationNames := coerceStringSlice(node["locations"])
		location := strings.Join(locationNames, ", ")

		remote := coerceBool(node["remote"])

		publishedAt := time.Now()
		if value := coerceString(node["publishedAt"]); value != "" {
			if t, err := time.Parse(time.RFC3339, value); err == nil {
				publishedAt = t
			}
		}

		tags := make([]string, 0)
		if tagSlice, ok := node["tags"].([]interface{}); ok {
			for _, tag := range tagSlice {
				if tagMap, ok := tag.(map[string]interface{}); ok {
					if name := strings.TrimSpace(coerceString(tagMap["name"])); name != "" {
						tags = append(tags, name)
					}
				}
			}
		}

		normalizedParts := []string{
			strings.ToLower(title),
			strings.ToLower(description),
			strings.ToLower(location),
		}
		if companyName != "" {
			normalizedParts = append(normalizedParts, strings.ToLower(companyName))
		}

		sourceKey := coerceString(node["id"])
		if sourceKey == "" && applyURL != "" {
			sourceKey = applyURL
		}

		postings = append(postings, jobPosting{
			Source:         "wellfound",
			SourceKey:      fmt.Sprintf("wellfound:%s", sourceKey),
			Title:          title,
			Company:        companyName,
			Location:       location,
			Remote:         remote,
			ApplyURL:       applyURL,
			Description:    description,
			PublishedAt:    publishedAt,
			Tags:           tags,
			NormalizedText: strings.Join(normalizedParts, " "),
		})
	}

	if len(postings) > limit {
		postings = postings[:limit]
	}

	return postings, nil
}

func (s *JobMatchingService) fetchUSAJobs(ctx context.Context, profile resumeProfile) ([]jobPosting, error) {
	base := strings.TrimSpace(s.cfg.USAJobsBaseURL)
	if base == "" {
		base = "https://data.usajobs.gov"
	}
	base = strings.TrimSuffix(base, "/")

	userAgent := strings.TrimSpace(s.cfg.USAJobsUserAgent)
	apiKey := strings.TrimSpace(s.cfg.USAJobsAPIKey)
	if userAgent == "" || apiKey == "" {
		return nil, fmt.Errorf("usajobs credentials not configured")
	}

	limit := s.cfg.MaxJobsPerProvider
	if limit <= 0 || limit > 50 {
		limit = 50
	}

	u, err := url.Parse(base + "/api/search")
	if err != nil {
		return nil, err
	}

	queryValues := u.Query()
	if len(profile.QueryTerms) > 0 {
		queryValues.Set("Keyword", strings.Join(profile.QueryTerms[:min(5, len(profile.QueryTerms))], " "))
	}
	if profile.PrimaryLocation != "" {
		queryValues.Set("LocationName", profile.PrimaryLocation)
	} else if profile.CountryHint != "" {
		queryValues.Set("CountrySubDivisionCode", strings.ToUpper(profile.CountryHint))
	}
	queryValues.Set("ResultsPerPage", strconv.Itoa(limit))
	u.RawQuery = queryValues.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Authorization-Key", apiKey)
	if host := strings.TrimSpace(s.cfg.USAJobsHost); host != "" {
		req.Header.Set("Host", host)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("usajobs request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	var payload struct {
		SearchResult struct {
			SearchResultItems []struct {
				MatchedObjectID         string `json:"MatchedObjectId"`
				MatchedObjectDescriptor struct {
					PositionTitle        string   `json:"PositionTitle"`
					OrganizationName     string   `json:"OrganizationName"`
					PositionURI          string   `json:"PositionURI"`
					ApplyURI             []string `json:"ApplyURI"`
					QualificationSummary string   `json:"QualificationSummary"`
					PublicationStartDate string   `json:"PublicationStartDate"`
					PositionLocation     []struct {
						LocationName string `json:"LocationName"`
					} `json:"PositionLocation"`
					UserArea struct {
						Details struct {
							RemoteIndicator   string `json:"RemoteIndicator"`
							TeleworkIndicator string `json:"TeleworkIndicator"`
						} `json:"Details"`
					} `json:"UserArea"`
				} `json:"MatchedObjectDescriptor"`
			} `json:"SearchResultItems"`
		} `json:"SearchResult"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	postings := make([]jobPosting, 0, len(payload.SearchResult.SearchResultItems))
	for _, item := range payload.SearchResult.SearchResultItems {
		descriptor := item.MatchedObjectDescriptor
		title := strings.TrimSpace(descriptor.PositionTitle)
		if title == "" {
			continue
		}

		company := strings.TrimSpace(descriptor.OrganizationName)

		locationParts := make([]string, 0, len(descriptor.PositionLocation))
		for _, loc := range descriptor.PositionLocation {
			if trimmed := strings.TrimSpace(loc.LocationName); trimmed != "" {
				locationParts = append(locationParts, trimmed)
			}
		}
		location := strings.Join(locationParts, ", ")

		applyURL := ""
		if len(descriptor.ApplyURI) > 0 {
			applyURL = strings.TrimSpace(descriptor.ApplyURI[0])
		}
		if applyURL == "" {
			applyURL = strings.TrimSpace(descriptor.PositionURI)
		}

		desc := sanitizeDescription(descriptor.QualificationSummary)

		remote := false
		remoteDetails := descriptor.UserArea.Details
		if strings.EqualFold(remoteDetails.RemoteIndicator, "yes") || strings.EqualFold(remoteDetails.TeleworkIndicator, "yes") {
			remote = true
		}

		publishedAt := time.Now()
		if descriptor.PublicationStartDate != "" {
			if t, err := time.Parse(time.RFC3339, descriptor.PublicationStartDate); err == nil {
				publishedAt = t
			} else if t, err := time.Parse("01/02/2006", descriptor.PublicationStartDate); err == nil {
				publishedAt = t
			}
		}

		normalizedParts := []string{
			strings.ToLower(title),
			strings.ToLower(desc),
			strings.ToLower(location),
			strings.ToLower(company),
		}

		sourceKey := strings.TrimSpace(item.MatchedObjectID)
		if sourceKey == "" {
			sourceKey = applyURL
		}

		postings = append(postings, jobPosting{
			Source:         "usajobs",
			SourceKey:      fmt.Sprintf("usajobs:%s", sourceKey),
			Title:          title,
			Company:        company,
			Location:       location,
			Remote:         remote,
			ApplyURL:       applyURL,
			Description:    desc,
			PublishedAt:    publishedAt,
			Tags:           nil,
			NormalizedText: strings.Join(normalizedParts, " "),
		})
	}

	if len(postings) > limit {
		postings = postings[:limit]
	}

	return postings, nil
}

func (s *JobMatchingService) fetchSimplifyJobs(ctx context.Context, profile resumeProfile) ([]jobPosting, error) {
	base := strings.TrimSpace(s.cfg.SimplifyBaseURL)
	if base == "" {
		return nil, fmt.Errorf("simplify base url not configured")
	}

	limit := s.cfg.MaxJobsPerProvider
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	u, err := url.Parse(base)
	if err != nil {
		return nil, err
	}

	query := u.Query()
	query.Set("limit", strconv.Itoa(limit))
	if len(profile.QueryTerms) > 0 {
		query.Set("search", strings.Join(profile.QueryTerms[:min(5, len(profile.QueryTerms))], " "))
	}
	if profile.CountryHint != "" {
		query.Set("country", strings.ToUpper(profile.CountryHint))
	}
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if key := strings.TrimSpace(s.cfg.SimplifyAPIKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("simplify request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	var root interface{}
	if err := json.NewDecoder(resp.Body).Decode(&root); err != nil {
		return nil, err
	}

	var jobsSlice []interface{}
	switch value := root.(type) {
	case []interface{}:
		jobsSlice = value
	case map[string]interface{}:
		if arr, ok := value["jobs"].([]interface{}); ok {
			jobsSlice = arr
		} else if arr, ok := value["data"].([]interface{}); ok {
			jobsSlice = arr
		}
	}

	postings := make([]jobPosting, 0, len(jobsSlice))
	for _, job := range jobsSlice {
		jobMap, ok := job.(map[string]interface{})
		if !ok {
			continue
		}

		title := strings.TrimSpace(coerceString(jobMap["title"]))
		if title == "" {
			continue
		}

		company := strings.TrimSpace(coerceString(jobMap["company"]))
		if company == "" {
			company = strings.TrimSpace(coerceString(extractNested(jobMap, "company", "name")))
		}

		location := strings.TrimSpace(coerceString(jobMap["location"]))
		if location == "" {
			locations := coerceStringSlice(jobMap["locations"])
			location = strings.Join(locations, ", ")
		}

		desc := sanitizeDescription(coerceString(jobMap["description"]))
		applyURL := strings.TrimSpace(coerceString(jobMap["applyUrl"]))
		if applyURL == "" {
			applyURL = strings.TrimSpace(coerceString(jobMap["url"]))
		}

		remote := coerceBool(jobMap["remote"])

		publishedAt := time.Now()
		if value := coerceString(jobMap["publishedAt"]); value != "" {
			if t, err := time.Parse(time.RFC3339, value); err == nil {
				publishedAt = t
			}
		}

		tags := coerceStringSlice(jobMap["tags"])

		normalizedParts := []string{
			strings.ToLower(title),
			strings.ToLower(desc),
			strings.ToLower(location),
			strings.ToLower(company),
		}

		sourceKey := coerceString(jobMap["id"])
		if sourceKey == "" {
			sourceKey = coerceString(jobMap["jobId"])
		}
		if sourceKey == "" {
			sourceKey = applyURL
		}

		postings = append(postings, jobPosting{
			Source:         "simplify",
			SourceKey:      fmt.Sprintf("simplify:%s", sourceKey),
			Title:          title,
			Company:        company,
			Location:       location,
			Remote:         remote,
			ApplyURL:       applyURL,
			Description:    desc,
			PublishedAt:    publishedAt,
			Tags:           tags,
			NormalizedText: strings.Join(normalizedParts, " "),
		})
	}

	if len(postings) > limit {
		postings = postings[:limit]
	}

	return postings, nil
}

func extractNested(value interface{}, keys ...string) interface{} {
	current := value
	for _, key := range keys {
		asMap, ok := current.(map[string]interface{})
		if !ok {
			return nil
		}
		var ok2 bool
		current, ok2 = asMap[key]
		if !ok2 {
			return nil
		}
	}
	return current
}

func coerceString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	case json.Number:
		return v.String()
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func coerceBool(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		lower := strings.ToLower(strings.TrimSpace(v))
		return lower == "true" || lower == "yes" || lower == "1" || lower == "y"
	case float64:
		return v != 0
	case float32:
		return v != 0
	case int:
		return v != 0
	case int64:
		return v != 0
	default:
		return false
	}
}

func coerceStringSlice(value interface{}) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s := strings.TrimSpace(coerceString(item)); s != "" {
				result = append(result, s)
				continue
			}
			if m, ok := item.(map[string]interface{}); ok {
				if name := strings.TrimSpace(coerceString(m["name"])); name != "" {
					result = append(result, name)
				}
			}
		}
		return result
	case map[string]interface{}:
		// handle cases where list is nested under "nodes" or similar keys
		if arr, ok := v["nodes"].([]interface{}); ok {
			return coerceStringSlice(arr)
		}
	case nil:
		return nil
	}
	return nil
}

func sanitizeDescription(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	withoutTags := htmlTagRegex.ReplaceAllString(raw, " ")
	unescaped := html.UnescapeString(withoutTags)
	cleaned := normalizeWhitespace(unescaped)
	if len(cleaned) > 600 {
		cleaned = cleaned[:600]
		if idx := strings.LastIndex(cleaned, " "); idx > 300 {
			cleaned = cleaned[:idx]
		}
		cleaned = strings.TrimSpace(cleaned) + "…"
	}
	return cleaned
}

func stripDiacritics(value string) string {
	if value == "" {
		return ""
	}

	normalized, _, err := transform.String(diacriticTransformer, value)
	if err != nil {
		normalized = value
	}
	return specialRuneReplacer.Replace(normalized)
}

func normalizeWhitespace(input string) string {
	if input == "" {
		return ""
	}
	fields := strings.Fields(input)
	return strings.Join(fields, " ")
}

func humanizeIdentifier(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	parts := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == '-' || r == '_' || r == '.'
	})
	for i, part := range parts {
		parts[i] = capitalize(part)
	}
	return strings.Join(parts, " ")
}

func capitalize(word string) string {
	if word == "" {
		return ""
	}
	runes := []rune(strings.ToLower(word))
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
func buildResumeProfile(data ResumeData, jobDescription string) resumeProfile {
	skillPhrases := splitSkills(data.Skills)
	skillTokens := make([]string, 0, len(skillPhrases))
	for _, phrase := range skillPhrases {
		skillTokens = append(skillTokens, strings.ToLower(strings.TrimSpace(phrase)))
	}

	tokenCounts := make(map[string]int)
	addTokens(tokenCounts, data.Summary)
	for _, exp := range data.Experiences {
		addTokens(tokenCounts, exp.Description)
		addTokens(tokenCounts, exp.JobTitle)
	}
	for _, project := range data.Projects {
		addTokens(tokenCounts, project.Description)
		addTokens(tokenCounts, project.Technologies)
	}
	addTokens(tokenCounts, jobDescription)

	keywordTokens := selectTopTokens(tokenCounts, maxKeywordCount)

	targetTitle := extractTargetTitle(jobDescription, data.Experiences)
	if targetTitle == "" {
		for _, exp := range data.Experiences {
			if strings.TrimSpace(exp.JobTitle) != "" {
				targetTitle = strings.TrimSpace(exp.JobTitle)
				break
			}
		}
	}

	locations := collectLocations(data)
	primaryLocation := ""
	if len(locations) > 0 {
		primaryLocation = locations[0]
	}

	resumeKeywords := uniqueStrings(append(append([]string{}, skillPhrases...), keywordTokens...))
	if len(resumeKeywords) > 12 {
		resumeKeywords = resumeKeywords[:12]
	}

	countryHint := normalizeCountryCode(detectCountryHint(locations, jobDescription))
	if countryHint == "" && strings.TrimSpace(data.Country) != "" {
		if code := normalizeCountryCode(data.Country); code != "" {
			countryHint = code
		}
	}
	if countryHint == "" && strings.TrimSpace(data.Address) != "" {
		if code := detectCountryFromString(data.Address); code != "" {
			countryHint = code
		}
	}
	if countryHint == "" {
		if code := detectCountryFromPhone(data.Phone); code != "" {
			countryHint = code
		}
	}
	queryTerms := deriveQueryTerms(targetTitle, skillPhrases, keywordTokens)

	remotePreference := false
	if strings.Contains(strings.ToLower(jobDescription), "remote") {
		remotePreference = true
	}
	if !remotePreference {
		for _, exp := range data.Experiences {
			if exp.Remote {
				remotePreference = true
				break
			}
		}
	}

	return resumeProfile{
		TargetTitle:      targetTitle,
		SkillPhrases:     skillPhrases,
		SkillTokens:      uniqueStrings(skillTokens),
		KeywordTokens:    uniqueStrings(keywordTokens),
		QueryTerms:       queryTerms,
		Locations:        locations,
		PrimaryLocation:  primaryLocation,
		ResumeKeywords:   resumeKeywords,
		CountryHint:      countryHint,
		RemotePreference: remotePreference,
	}
}

func splitSkills(skills string) []string {
	if strings.TrimSpace(skills) == "" {
		return []string{}
	}
	parts := strings.Split(skills, ",")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return cleaned
}

func addTokens(counter map[string]int, input string) {
	for _, token := range tokenizeText(input) {
		counter[token]++
	}
}

func tokenizeText(input string) []string {
	if strings.TrimSpace(input) == "" {
		return []string{}
	}
	lower := strings.ToLower(input)
	cleaned := nonAlphaNumericRegex.ReplaceAllString(lower, " ")
	fields := strings.Fields(cleaned)
	tokens := make([]string, 0, len(fields))
	for _, field := range fields {
		if len(field) >= 3 {
			tokens = append(tokens, field)
		}
	}
	return tokens
}

func selectTopTokens(counter map[string]int, max int) []string {
	type entry struct {
		token string
		count int
	}
	entries := make([]entry, 0, len(counter))
	for token, count := range counter {
		entries = append(entries, entry{token: token, count: count})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].count == entries[j].count {
			return entries[i].token < entries[j].token
		}
		return entries[i].count > entries[j].count
	})

	limit := max
	if limit <= 0 || limit > len(entries) {
		limit = len(entries)
	}
	tokens := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		tokens = append(tokens, entries[i].token)
	}
	return tokens
}

func deriveQueryTerms(targetTitle string, skillPhrases, keywordTokens []string) []string {
	terms := make([]string, 0, 8)
	for _, token := range tokenizeText(targetTitle) {
		terms = append(terms, token)
	}
	for _, skill := range skillPhrases {
		skillToken := strings.ToLower(strings.TrimSpace(skill))
		if skillToken != "" {
			terms = append(terms, skillToken)
		}
		if len(terms) >= 6 {
			break
		}
	}
	if len(terms) < 6 {
		for _, kw := range keywordTokens {
			terms = append(terms, kw)
			if len(terms) >= 6 {
				break
			}
		}
	}
	return uniqueStrings(terms)
}

func collectLocations(data ResumeData) []string {
	locations := make([]string, 0)
	seen := make(map[string]struct{})

	appendUnique := func(value string) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return
		}
		normalized := strings.ToLower(trimmed)
		if _, exists := seen[normalized]; exists {
			return
		}
		seen[normalized] = struct{}{}
		locations = append(locations, trimmed)
	}

	for _, exp := range data.Experiences {
		appendUnique(strings.Join([]string{exp.City, exp.State}, " "))
	}

	for _, edu := range data.Education {
		appendUnique(edu.Location)
	}

	if data.Address != "" {
		appendUnique(data.Address)
	}
	if data.City != "" || data.State != "" {
		appendUnique(strings.Join([]string{data.City, data.State}, " "))
	}
	if data.Country != "" {
		appendUnique(data.Country)
	}

	return locations
}
func detectCountryHint(locations []string, jobDescription string) string {
	if code := detectCountryFromStrings(locations); code != "" {
		return code
	}
	if code := detectCountryFromStrings([]string{jobDescription}); code != "" {
		return code
	}
	return ""
}

func detectCountryFromStrings(inputs []string) string {
	for _, input := range inputs {
		if code := detectCountryFromString(input); code != "" {
			return code
		}
	}
	return ""
}

func detectCountryFromString(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	lower := strings.ToLower(trimmed)
	for code, aliases := range countryAliases {
		for _, alias := range aliases {
			if strings.Contains(lower, alias) {
				return normalizeCountryCode(code)
			}
		}
	}

	tokens := tokenizeLocationParts(trimmed)
	for _, token := range tokens {
		upper := strings.ToUpper(token)
		if code, ok := regionToCountry[upper]; ok {
			return normalizeCountryCode(code)
		}
		normalizedToken := strings.ToLower(token)
		if code, ok := cityToCountry[normalizedToken]; ok {
			return normalizeCountryCode(code)
		}
	}

	normalized := strings.ToLower(stripDiacritics(trimmed))
	if code, ok := cityToCountry[normalized]; ok {
		return normalizeCountryCode(code)
	}
	for city, code := range cityToCountry {
		if strings.Contains(normalized, city) {
			return normalizeCountryCode(code)
		}
	}

	return ""
}

func normalizeCountryCode(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	switch code {
	case "", "undefined":
		return ""
	case "gb", "uk", "eng", "en-gb":
		return "uk"
	case "us", "usa", "en-us":
		return "us"
	case "au", "aus":
		return "au"
	case "ca", "can":
		return "ca"
	case "cn":
		return "cn"
	default:
		return code
	}
}
func detectCountryFromPhone(phone string) string {
	if strings.TrimSpace(phone) == "" {
		return ""
	}
	digits := make([]rune, 0, len(phone))
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
		}
	}
	if len(digits) == 0 {
		return ""
	}
	if digits[0] == '1' {
		return "us"
	}
	trimmed := strings.TrimSpace(phone)
	if strings.HasPrefix(trimmed, "+1") {
		return "us"
	}
	return ""
}

func containsCountryIndicator(value, country string) bool {
	country = normalizeCountryCode(country)
	if country == "" {
		return false
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}

	lower := strings.ToLower(trimmed)
	if aliases, ok := countryAliases[country]; ok {
		for _, alias := range aliases {
			if strings.Contains(lower, alias) {
				return true
			}
		}
	}

	tokens := tokenizeLocationParts(trimmed)
	for _, token := range tokens {
		upper := strings.ToUpper(token)
		if code, ok := regionToCountry[upper]; ok && normalizeCountryCode(code) == country {
			return true
		}
		normalizedToken := strings.ToLower(token)
		if code, ok := cityToCountry[normalizedToken]; ok && normalizeCountryCode(code) == country {
			return true
		}
	}

	normalized := strings.ToLower(stripDiacritics(trimmed))
	if code, ok := cityToCountry[normalized]; ok && normalizeCountryCode(code) == country {
		return true
	}
	for city, code := range cityToCountry {
		if strings.Contains(normalized, city) && normalizeCountryCode(code) == country {
			return true
		}
	}

	return false
}

func tokenizeLocationParts(value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	normalized := stripDiacritics(trimmed)
	parts := strings.FieldsFunc(normalized, func(r rune) bool {
		switch r {
		case ',', '|', '/', '\\', '-', '(', ')':
			return true
		}
		return unicode.IsSpace(r)
	})
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		token := strings.TrimSpace(part)
		if token != "" {
			cleaned = append(cleaned, token)
		}
	}
	return cleaned
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	uniques := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		normalized := strings.ToLower(trimmed)
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		uniques = append(uniques, trimmed)
	}
	return uniques
}
func extractTargetTitle(jobDescription string, experiences []ResumeExperience) string {
	text := strings.TrimSpace(jobDescription)
	if text != "" {
		lines := strings.Split(text, "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			lower := strings.ToLower(trimmed)
			switch {
			case strings.HasPrefix(lower, "position:"):
				return strings.TrimSpace(trimmed[len("position:"):])
			case strings.HasPrefix(lower, "title:"):
				return strings.TrimSpace(trimmed[len("title:"):])
			case strings.HasPrefix(lower, "role:"):
				return strings.TrimSpace(trimmed[len("role:"):])
			}
		}

		words := strings.Fields(text)
		if len(words) > 0 && len(words) <= 6 {
			candidate := strings.Join(words[:min(6, len(words))], " ")
			if len(candidate) <= 48 {
				return candidate
			}
		}
	}

	for _, exp := range experiences {
		if strings.TrimSpace(exp.JobTitle) != "" {
			return strings.TrimSpace(exp.JobTitle)
		}
	}

	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func deduplicatePostings(postings []jobPosting) []jobPosting {
	seen := make(map[string]struct{}, len(postings))
	uniques := make([]jobPosting, 0, len(postings))
	for _, posting := range postings {
		key := posting.Source + ":" + posting.SourceKey
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		uniques = append(uniques, posting)
	}
	return uniques
}
func filterPostingsByCountry(country string, postings []jobPosting) []jobPosting {
	country = normalizeCountryCode(country)
	if country == "" {
		return postings
	}

	filtered := make([]jobPosting, 0, len(postings))
	worldwide := make([]jobPosting, 0, len(postings))

	for _, posting := range postings {
		if posting.Remote {
			if matchesCountryHint(posting, country) {
				filtered = append(filtered, posting)
				continue
			}
			if isRemoteWorldwide(posting) {
				worldwide = append(worldwide, posting)
			}
			continue
		}

		if matchesCountryHint(posting, country) {
			filtered = append(filtered, posting)
		}
	}

	if len(filtered) > 0 {
		return filtered
	}
	if len(worldwide) > 0 {
		return worldwide
	}

	return filtered
}
func matchesCountryHint(posting jobPosting, country string) bool {
	country = normalizeCountryCode(country)
	if country == "" {
		return false
	}

	if posting.Remote {
		if containsCountryIndicator(posting.Location, country) {
			return true
		}
		if containsCountryIndicator(posting.Category, country) {
			return true
		}
		for _, tag := range posting.Tags {
			if containsCountryIndicator(tag, country) {
				return true
			}
		}
		if containsCountryIndicator(posting.Description, country) {
			return true
		}
		return false
	}

	if containsCountryIndicator(posting.Location, country) {
		return true
	}
	if containsCountryIndicator(posting.Category, country) {
		return true
	}
	for _, tag := range posting.Tags {
		if containsCountryIndicator(tag, country) {
			return true
		}
	}
	return false
}
func isRemoteWorldwide(posting jobPosting) bool {
	fields := []string{posting.Location, posting.Description}
	fields = append(fields, posting.Tags...)

	regionalKeywords := []string{"europe", "emea", "apac", "asia", "latam", "canada", "germany", "france", "italy", "spain", "uk", "united kingdom", "ireland", "india", "singapore", "australia", "new zealand"}
	hasSpecificRegion := false
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		if trimmed == "" {
			continue
		}
		if detectCountryFromString(trimmed) != "" {
			hasSpecificRegion = true
			break
		}
		lower := strings.ToLower(trimmed)
		for _, keyword := range regionalKeywords {
			if strings.Contains(lower, keyword) {
				hasSpecificRegion = true
				break
			}
		}
		if hasSpecificRegion {
			break
		}
	}

	if hasSpecificRegion {
		return false
	}

	combined := strings.ToLower(strings.Join(fields, " "))
	worldwideKeywords := []string{"worldwide", "global", "anywhere", "remote-first", "distributed", "timezone agnostic", "time zone agnostic"}
	for _, keyword := range worldwideKeywords {
		if strings.Contains(combined, keyword) {
			return true
		}
	}

	trimmedLocation := strings.TrimSpace(strings.ToLower(posting.Location))
	if trimmedLocation == "" || trimmedLocation == "remote" {
		return true
	}

	return false
}
func rankJobMatches(profile resumeProfile, postings []jobPosting, limit int) []JobMatch {
	matches := make([]JobMatch, 0, len(postings))
	for _, posting := range postings {
		score, matchedSkills, matchedKeywords := scorePosting(profile, posting)
		if score <= 0 {
			continue
		}
		matches = append(matches, makeJobMatch(posting, score, matchedSkills, matchedKeywords))
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score == matches[j].Score {
			return matches[i].PublishedAt.After(matches[j].PublishedAt)
		}
		return matches[i].Score > matches[j].Score
	})

	if len(matches) > limit {
		matches = matches[:limit]
	}

	if len(matches) == 0 && len(postings) > 0 {
		sort.Slice(postings, func(i, j int) bool {
			return postings[i].PublishedAt.After(postings[j].PublishedAt)
		})
		fallbackCount := min(limit, len(postings))
		matches = make([]JobMatch, 0, fallbackCount)
		for i := 0; i < fallbackCount; i++ {
			matches = append(matches, makeJobMatch(postings[i], 0, nil, nil))
		}
	}

	return matches
}
func scorePosting(profile resumeProfile, posting jobPosting) (float64, []string, []string) {
	text := posting.NormalizedText
	if text == "" {
		text = strings.ToLower(strings.Join([]string{posting.Title, posting.Description, posting.Location, strings.Join(posting.Tags, " ")}, " "))
	}

	skillMatches := make([]string, 0)
	keywordMatches := make([]string, 0)
	score := 0.0

	seenSkill := make(map[string]struct{})
	for _, phrase := range profile.SkillPhrases {
		normalized := strings.ToLower(strings.TrimSpace(phrase))
		if normalized == "" {
			continue
		}
		if strings.Contains(text, normalized) {
			if _, exists := seenSkill[normalized]; !exists {
				seenSkill[normalized] = struct{}{}
				skillMatches = append(skillMatches, phrase)
				score += 3.0
			}
		}
	}

	seenKeyword := make(map[string]struct{})
	for _, token := range profile.KeywordTokens {
		if token == "" {
			continue
		}
		if strings.Contains(text, token) {
			if _, exists := seenKeyword[token]; !exists {
				seenKeyword[token] = struct{}{}
				keywordMatches = append(keywordMatches, token)
				score += 1.0
			}
		}
	}

	if profile.TargetTitle != "" {
		target := strings.ToLower(profile.TargetTitle)
		title := strings.ToLower(posting.Title)
		if strings.Contains(title, target) {
			if _, exists := seenKeyword[target]; !exists {
				keywordMatches = append(keywordMatches, strings.TrimSpace(profile.TargetTitle))
			}
			score += 4.0
		}
	}

	if profile.PrimaryLocation != "" {
		if strings.Contains(strings.ToLower(posting.Location), strings.ToLower(profile.PrimaryLocation)) {
			score += 2.0
		}
	}

	if posting.Remote {
		if profile.RemotePreference {
			score += 1.5
		} else {
			score += 0.5
		}
	}

	return score, skillMatches, keywordMatches
}

func makeJobMatch(posting jobPosting, score float64, matchedSkills, matchedKeywords []string) JobMatch {
	return JobMatch{
		Source:          posting.Source,
		SourceID:        posting.SourceKey,
		Title:           posting.Title,
		Company:         posting.Company,
		Location:        posting.Location,
		Remote:          posting.Remote,
		ApplyURL:        posting.ApplyURL,
		Description:     posting.Description,
		JobType:         posting.JobType,
		Category:        posting.Category,
		PublishedAt:     posting.PublishedAt,
		Score:           score,
		ScoreLabel:      scoreToLabel(score),
		MatchedSkills:   matchedSkills,
		MatchedKeywords: matchedKeywords,
	}
}

func scoreToLabel(score float64) string {
	switch {
	case score >= 12:
		return "excellent"
	case score >= 7:
		return "strong"
	case score >= 4:
		return "good"
	case score > 0:
		return "emerging"
	default:
		return "unscored"
	}
}
