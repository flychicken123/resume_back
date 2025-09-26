package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type JobBoardConfig struct {
	GreenhouseBoardTokens       []string
	LeverCompanySlugs           []string
	SmartRecruitersCompanySlugs []string
	RemotiveEnabled             bool
	ArbeitnowEnabled            bool
	SmartRecruitersEnabled      bool
	MuseEnabled                 bool
	WellfoundEnabled            bool
	USAJobsEnabled              bool
	SimplifyEnabled             bool
	MaxJobsPerProvider          int
	RequestTimeout              time.Duration
	GreenhouseBaseURL           string
	LeverBaseURL                string
	SmartRecruitersBaseURL      string
	RemotiveBaseURL             string
	ArbeitnowBaseURL            string
	MuseBaseURL                 string
	MuseAPIKey                  string
	WellfoundBaseURL            string
	WellfoundAPIToken           string
	USAJobsBaseURL              string
	USAJobsHost                 string
	USAJobsUserAgent            string
	USAJobsAPIKey               string
	SimplifyBaseURL             string
	SimplifyAPIKey              string
}

func GetJobBoardConfig() JobBoardConfig {
	timeoutSeconds, err := strconv.Atoi(getEnv("JOB_MATCH_HTTP_TIMEOUT_SEC", "6"))
	if err != nil || timeoutSeconds <= 0 {
		timeoutSeconds = 6
	}

	maxJobs, err := strconv.Atoi(getEnv("JOB_MATCH_MAX_PER_PROVIDER", "50"))
	if err != nil || maxJobs <= 0 {
		maxJobs = 50
	}

	return JobBoardConfig{
		GreenhouseBoardTokens:       parseListEnv("GREENHOUSE_BOARD_TOKENS"),
		LeverCompanySlugs:           parseListEnv("LEVER_COMPANY_SLUGS"),
		SmartRecruitersCompanySlugs: parseListEnv("SMARTRECRUITERS_COMPANY_SLUGS"),
		RemotiveEnabled:             parseBoolEnv("REMOTIVE_API_ENABLED", true),
		ArbeitnowEnabled:            parseBoolEnv("ARBEITNOW_API_ENABLED", true),
		SmartRecruitersEnabled:      parseBoolEnv("SMARTRECRUITERS_API_ENABLED", true),
		MuseEnabled:                 parseBoolEnv("MUSE_API_ENABLED", true),
		WellfoundEnabled:            parseBoolEnv("WELLFOUND_API_ENABLED", false),
		USAJobsEnabled:              parseBoolEnv("USAJOBS_API_ENABLED", false),
		SimplifyEnabled:             parseBoolEnv("SIMPLIFY_API_ENABLED", false),
		MaxJobsPerProvider:          maxJobs,
		RequestTimeout:              time.Duration(timeoutSeconds) * time.Second,
		GreenhouseBaseURL:           getEnv("GREENHOUSE_BASE_URL", "https://boards-api.greenhouse.io"),
		LeverBaseURL:                getEnv("LEVER_BASE_URL", "https://api.lever.co"),
		SmartRecruitersBaseURL:      getEnv("SMARTRECRUITERS_BASE_URL", "https://api.smartrecruiters.com"),
		RemotiveBaseURL:             getEnv("REMOTIVE_BASE_URL", "https://remotive.com"),
		ArbeitnowBaseURL:            getEnv("ARBEITNOW_BASE_URL", "https://www.arbeitnow.com"),
		MuseBaseURL:                 getEnv("MUSE_BASE_URL", "https://www.themuse.com"),
		MuseAPIKey:                  getEnv("MUSE_API_KEY", ""),
		WellfoundBaseURL:            getEnv("WELLFOUND_BASE_URL", "https://wellfound.com/graphql"),
		WellfoundAPIToken:           getEnv("WELLFOUND_API_TOKEN", ""),
		USAJobsBaseURL:              getEnv("USAJOBS_BASE_URL", "https://data.usajobs.gov"),
		USAJobsHost:                 getEnv("USAJOBS_HOST", ""),
		USAJobsUserAgent:            getEnv("USAJOBS_USER_AGENT", ""),
		USAJobsAPIKey:               getEnv("USAJOBS_API_KEY", ""),
		SimplifyBaseURL:             getEnv("SIMPLIFY_BASE_URL", ""),
		SimplifyAPIKey:              getEnv("SIMPLIFY_API_KEY", ""),
	}
}

func parseListEnv(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return []string{}
	}

	parts := strings.Split(raw, ",")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return cleaned
}

func parseBoolEnv(key string, defaultValue bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return defaultValue
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return defaultValue
	}
}
