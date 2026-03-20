package services

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"resumeai/utils"
)

// SkillInferenceLLM provides LLM-powered skill inference from resume content.
type SkillInferenceLLM struct {
	cache   *skillInferenceCache
	dbCache SkillDBCache
	logger  *utils.Logger
}

// SkillDBCache is an interface for persisting inferred skills to DB.
type SkillDBCache interface {
	GetSkills(resumeHash string) ([]string, error)
	UpsertSkills(resumeHash string, skills []string) error
}

// skillInferenceCache caches LLM inference results to avoid repeated API calls.
type skillInferenceCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	ttl     time.Duration
}

type cacheEntry struct {
	skills    []string
	createdAt time.Time
}

// ResumeContent holds the resume data for skill inference.
type ResumeContent struct {
	Position   string
	Summary    string
	Experience string
	Education  string
	Skills     []string
	Projects   string
}

// LLMSkillInferenceResult represents the parsed LLM response.
type LLMSkillInferenceResult struct {
	ExplicitSkills  []string `json:"explicit_skills"`
	InferredSkills  []string `json:"inferred_skills"`
	SkillCategories map[string][]string `json:"skill_categories"`
}

// NewSkillInferenceLLM creates a new LLM-powered skill inference service.
func NewSkillInferenceLLM(logger *utils.Logger) *SkillInferenceLLM {
	if logger == nil {
		logger = utils.NewLogger()
	}
	return &SkillInferenceLLM{
		cache: &skillInferenceCache{
			entries: make(map[string]*cacheEntry),
			ttl:     30 * time.Minute,
		},
		logger: logger,
	}
}

// InferSkillsFromResume uses Gemini to intelligently extract and infer skills
// from resume content. It analyzes the full context including projects,
// experience descriptions, and job titles to infer implicit skills.
func (s *SkillInferenceLLM) InferSkillsFromResume(ctx context.Context, content ResumeContent) ([]string, error) {
	// Generate cache key from content hash
	cacheKey := s.generateCacheKey(content)

	// Check in-memory cache first
	if cached := s.cache.get(cacheKey); cached != nil {
		return cached, nil
	}

	// Check DB cache
	if s.dbCache != nil {
		if dbSkills, err := s.dbCache.GetSkills(cacheKey); err == nil && len(dbSkills) > 0 {
			s.cache.set(cacheKey, dbSkills) // warm in-memory cache
			return dbSkills, nil
		}
	}

	// Build prompt for LLM
	prompt := s.buildSkillInferencePrompt(content)

	// Call Gemini
	response, err := CallGeminiFlash(prompt)
	if err != nil {
		s.logger.Warn("LLM skill inference failed, falling back to rule-based", map[string]interface{}{
			"error": err.Error(),
		})
		// Fall back to rule-based inference
		return s.fallbackInference(content), nil
	}

	// Parse LLM response
	skills, err := s.parseSkillResponse(response)
	if err != nil {
		s.logger.Warn("Failed to parse LLM skill response, falling back to rule-based", map[string]interface{}{
			"error":    err.Error(),
			"response": truncateString(response, 200),
		})
		return s.fallbackInference(content), nil
	}

	// Merge with explicit skills and canonicalize
	allSkills := s.mergeAndCanonicalizeSkills(content.Skills, skills)

	// Cache the result (in-memory + DB)
	s.cache.set(cacheKey, allSkills)
	if s.dbCache != nil {
		_ = s.dbCache.UpsertSkills(cacheKey, allSkills)
	}

	return allSkills, nil
}

func (s *SkillInferenceLLM) buildSkillInferencePrompt(content ResumeContent) string {
	var sb strings.Builder

	sb.WriteString(`You are an expert technical recruiter analyzing a resume. Your task is to extract ALL technical skills - both explicitly mentioned and implicitly demonstrated through projects and experience.

IMPORTANT: Infer skills from context. For example:
- "Built a REST API with Express.js and PostgreSQL" implies: backend, api, database, node, express, postgresql
- "Deployed to AWS using Docker containers" implies: cloud, aws, docker, devops
- "Created responsive UI with React" implies: frontend, react, javascript

Analyze the following resume content and extract skills:

`)

	if content.Position != "" {
		sb.WriteString(fmt.Sprintf("TARGET POSITION: %s\n\n", content.Position))
	}

	if content.Summary != "" {
		sb.WriteString(fmt.Sprintf("PROFESSIONAL SUMMARY:\n%s\n\n", content.Summary))
	}

	if content.Experience != "" {
		sb.WriteString(fmt.Sprintf("WORK EXPERIENCE:\n%s\n\n", content.Experience))
	}

	if content.Projects != "" {
		sb.WriteString(fmt.Sprintf("PROJECTS:\n%s\n\n", content.Projects))
	}

	if content.Education != "" {
		sb.WriteString(fmt.Sprintf("EDUCATION:\n%s\n\n", content.Education))
	}

	if len(content.Skills) > 0 {
		sb.WriteString(fmt.Sprintf("LISTED SKILLS: %s\n\n", strings.Join(content.Skills, ", ")))
	}

	sb.WriteString(`Return a JSON object with the following structure:
{
  "explicit_skills": ["skill1", "skill2"],
  "inferred_skills": ["skill3", "skill4"],
  "skill_categories": {
    "frontend": ["react", "typescript"],
    "backend": ["node", "express", "postgresql"],
    "devops": ["docker", "aws"],
    "cloud": ["aws"],
    "database": ["postgresql"],
    "testing": ["jest"],
    "api": ["rest", "graphql"]
  }
}

Rules:
1. explicit_skills: Skills directly mentioned in the resume
2. inferred_skills: Skills implied by projects/experience but not explicitly listed
3. skill_categories: Group all skills (explicit + inferred) by category
4. Use lowercase for all skill names
5. Be comprehensive - extract ALL relevant technical skills
6. Include soft skills categories like "leadership", "communication" if demonstrated

Return ONLY the JSON object, no additional text.`)

	return sb.String()
}

func (s *SkillInferenceLLM) parseSkillResponse(response string) ([]string, error) {
	// Clean up response - extract JSON if wrapped in markdown
	response = strings.TrimSpace(response)
	if strings.HasPrefix(response, "```json") {
		response = strings.TrimPrefix(response, "```json")
		response = strings.TrimSuffix(response, "```")
		response = strings.TrimSpace(response)
	} else if strings.HasPrefix(response, "```") {
		response = strings.TrimPrefix(response, "```")
		response = strings.TrimSuffix(response, "```")
		response = strings.TrimSpace(response)
	}

	var result LLMSkillInferenceResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Collect all unique skills
	skillSet := make(map[string]bool)

	for _, skill := range result.ExplicitSkills {
		skillSet[strings.ToLower(strings.TrimSpace(skill))] = true
	}

	for _, skill := range result.InferredSkills {
		skillSet[strings.ToLower(strings.TrimSpace(skill))] = true
	}

	for _, skills := range result.SkillCategories {
		for _, skill := range skills {
			skillSet[strings.ToLower(strings.TrimSpace(skill))] = true
		}
	}

	// Convert to sorted slice
	skills := make([]string, 0, len(skillSet))
	for skill := range skillSet {
		if skill != "" {
			skills = append(skills, skill)
		}
	}
	sort.Strings(skills)

	return skills, nil
}

func (s *SkillInferenceLLM) mergeAndCanonicalizeSkills(explicitSkills, llmSkills []string) []string {
	skillSet := make(map[string]bool)

	// Add canonicalized explicit skills
	for _, skill := range explicitSkills {
		canonical := GetCanonicalSkill(skill)
		skillSet[canonical] = true
	}

	// Add LLM-extracted skills
	for _, skill := range llmSkills {
		canonical := GetCanonicalSkill(skill)
		skillSet[canonical] = true
	}

	// Convert to sorted slice
	result := make([]string, 0, len(skillSet))
	for skill := range skillSet {
		if skill != "" {
			result = append(result, skill)
		}
	}
	sort.Strings(result)

	return result
}

func (s *SkillInferenceLLM) fallbackInference(content ResumeContent) []string {
	// Use rule-based inference as fallback
	allSkills := make([]string, 0, len(content.Skills)*2)
	allSkills = append(allSkills, content.Skills...)

	// Extract skills from text content
	textContent := strings.Join([]string{
		content.Summary,
		content.Experience,
		content.Projects,
		content.Education,
	}, " ")

	extractedSkills := ExtractSkillsFromText(textContent)
	allSkills = append(allSkills, extractedSkills...)

	// Use rule-based inference
	return ExpandSkillsWithInference(allSkills)
}

func (s *SkillInferenceLLM) generateCacheKey(content ResumeContent) string {
	// Create a simple hash from content
	combined := strings.Join([]string{
		content.Position,
		content.Summary,
		content.Experience,
		content.Education,
		content.Projects,
		strings.Join(content.Skills, ","),
	}, "|")

	// Simple hash - just use first 100 chars + length
	if len(combined) > 100 {
		return fmt.Sprintf("%s_%d", combined[:100], len(combined))
	}
	return combined
}

func (c *skillInferenceCache) get(key string) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[key]
	if !exists {
		return nil
	}

	if time.Since(entry.createdAt) > c.ttl {
		return nil
	}

	// Return a copy to prevent mutation
	result := make([]string, len(entry.skills))
	copy(result, entry.skills)
	return result
}

func (c *skillInferenceCache) set(key string, skills []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Clean up old entries periodically
	if len(c.entries) > 1000 {
		now := time.Now()
		for k, v := range c.entries {
			if now.Sub(v.createdAt) > c.ttl {
				delete(c.entries, k)
			}
		}
	}

	// Store a copy to prevent mutation
	skillsCopy := make([]string, len(skills))
	copy(skillsCopy, skills)

	c.entries[key] = &cacheEntry{
		skills:    skillsCopy,
		createdAt: time.Now(),
	}
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// Global instance for easy access
var globalSkillInferenceLLM *SkillInferenceLLM
var skillInferenceLLMOnce sync.Once

// GetSkillInferenceLLM returns the global skill inference LLM instance.
func GetSkillInferenceLLM() *SkillInferenceLLM {
	skillInferenceLLMOnce.Do(func() {
		globalSkillInferenceLLM = NewSkillInferenceLLM(utils.NewLogger())
	})
	return globalSkillInferenceLLM
}

// SetSkillInferenceDBCache wires a DB cache into the global skill inference instance.
func SetSkillInferenceDBCache(dbCache SkillDBCache) {
	GetSkillInferenceLLM().dbCache = dbCache
}
