package services

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type ResumeAnalysisFacts struct {
	SectionCoverage ResumeSectionCoverage  `json:"section_coverage"`
	Counts          ResumeAnalysisCounts   `json:"counts"`
	MissingFields   []ResumeAnalysisIssue  `json:"missing_fields"`
	QualityIssues   []ResumeAnalysisIssue  `json:"quality_issues"`
	StrengthSignals []ResumeAnalysisSignal `json:"strength_signals"`
}

type ResumeSectionCoverage struct {
	HasSummary        bool `json:"has_summary"`
	HasExperience     bool `json:"has_experience"`
	HasSkills         bool `json:"has_skills"`
	HasEducation      bool `json:"has_education"`
	HasProjects       bool `json:"has_projects"`
	HasJobDescription bool `json:"has_job_description"`
}

type ResumeAnalysisCounts struct {
	ExperienceCount        int `json:"experience_count"`
	EducationCount         int `json:"education_count"`
	ProjectCount           int `json:"project_count"`
	SkillCount             int `json:"skill_count"`
	BulletCount            int `json:"bullet_count"`
	BulletsWithMetrics     int `json:"bullets_with_metrics"`
	BulletsWithoutMetrics  int `json:"bullets_without_metrics"`
	WeakVerbBulletCount    int `json:"weak_verb_bullet_count"`
	LongBulletCount        int `json:"long_bullet_count"`
	MissingExperienceDates int `json:"missing_experience_dates"`
}

type ResumeAnalysisIssue struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Section  string `json:"section"`
	Message  string `json:"message"`
	Evidence string `json:"evidence,omitempty"`
	Action   string `json:"action"`
}

type ResumeAnalysisSignal struct {
	Code     string `json:"code"`
	Section  string `json:"section"`
	Message  string `json:"message"`
	Evidence string `json:"evidence,omitempty"`
}

var (
	resumeMetricPattern = regexp.MustCompile(`(?i)(\b\d+(?:[.,]\d+)?%?\b|\$\s?\d+|\b(?:million|billion|thousand|ms|seconds?|minutes?|hours?|days?|weeks?|months?|years?|users?|customers?|requests?|transactions?|uptime|latency|cost|revenue|savings)\b)`)
	weakBulletPattern   = regexp.MustCompile(`(?i)^\s*(?:[-*]\s*)?(worked on|responsible for|helped|assisted|participated in|involved in|handled)\b`)
)

func AnalyzeResumeDeterministicFacts(resumeData map[string]interface{}, jobDescription string) ResumeAnalysisFacts {
	facts := ResumeAnalysisFacts{}

	summary := strings.TrimSpace(toAnalysisString(resumeData["summary"]))
	skills := splitAnalysisList(resumeData["skills"])
	experiences := analysisMapSlice(resumeData["experiences"])
	education := analysisMapSlice(resumeData["education"])
	projects := analysisMapSlice(resumeData["projects"])
	jobDescription = strings.TrimSpace(jobDescription)
	if jobDescription == "" {
		jobDescription = strings.TrimSpace(toAnalysisString(resumeData["jobDescription"]))
	}

	facts.SectionCoverage = ResumeSectionCoverage{
		HasSummary:        summary != "",
		HasExperience:     hasMeaningfulAnalysisEntry(experiences, "jobTitle", "company", "description"),
		HasSkills:         len(skills) > 0,
		HasEducation:      hasMeaningfulAnalysisEntry(education, "degree", "school", "field", "graduationYear"),
		HasProjects:       hasMeaningfulAnalysisEntry(projects, "projectName", "description", "technologies"),
		HasJobDescription: jobDescription != "",
	}
	facts.Counts.ExperienceCount = countMeaningfulAnalysisEntries(experiences, "jobTitle", "company", "description")
	facts.Counts.EducationCount = countMeaningfulAnalysisEntries(education, "degree", "school", "field", "graduationYear")
	facts.Counts.ProjectCount = countMeaningfulAnalysisEntries(projects, "projectName", "description", "technologies")
	facts.Counts.SkillCount = len(skills)

	if !facts.SectionCoverage.HasSummary {
		facts.MissingFields = append(facts.MissingFields, analysisIssue("missing_summary", "high", "summary", "Professional summary is missing.", "", "Add a 2-3 sentence summary that names the target role, core strengths, and strongest evidence."))
	} else {
		facts.StrengthSignals = append(facts.StrengthSignals, analysisSignal("summary_present", "summary", "Professional summary is present.", truncateAnalysisText(summary, 160)))
	}
	if !facts.SectionCoverage.HasSkills {
		facts.MissingFields = append(facts.MissingFields, analysisIssue("missing_skills_section", "high", "skills", "Dedicated skills section is missing or empty.", "", "Add a skills section grouped by languages, frameworks, cloud/tools, and domain skills."))
	}
	if !facts.SectionCoverage.HasExperience {
		facts.MissingFields = append(facts.MissingFields, analysisIssue("missing_experience_section", "high", "experience", "Work experience section is missing or empty.", "", "Add at least one role with title, company, dates, location, and achievement bullets."))
	}
	if !facts.SectionCoverage.HasEducation {
		facts.MissingFields = append(facts.MissingFields, analysisIssue("missing_education_section", "medium", "education", "Education section is missing or empty.", "", "Add degree, school, field, graduation date, and GPA/honors only if useful."))
	}
	if !facts.SectionCoverage.HasJobDescription {
		facts.QualityIssues = append(facts.QualityIssues, analysisIssue("missing_target_job_description", "medium", "targeting", "No target job description is available for tailoring.", "", "Paste a target job description before asking for job-specific optimization."))
	}

	scanExperienceFacts(experiences, &facts)
	scanProjectFacts(projects, &facts)
	sortAnalysisFacts(&facts)
	return facts
}

func BuildResumeAnalysisPrompt(resumeData map[string]interface{}, jobDescription string) string {
	facts := AnalyzeResumeDeterministicFacts(resumeData, jobDescription)
	factsJSON, _ := json.MarshalIndent(facts, "", "  ")
	resumeText := buildResumeAnalysisText(resumeData)

	return fmt.Sprintf(`You are a senior technical recruiter and resume quality analyst.

Analyze the resume using the deterministic facts first, then explain the highest-impact priorities.
Use only evidence from the resume and the deterministic facts. Do not invent employers, degrees, dates, metrics, or skills.
Do not say you already analyzed this resume before. Analyze the current resume state.

Return concise Markdown with exactly these sections:
1. Overall assessment: 2-3 sentences with the strongest evidence and biggest risk.
2. Priority fixes: 3-5 ranked fixes. Each item must include severity, evidence, why it matters, and the concrete next action.
3. Strengths to preserve: 2-4 bullets with evidence.
4. ATS and recruiter impact: explain how missing fields, metrics, and structure affect screening.
5. Next actions: 3 short commands the user can take in the builder.

DETERMINISTIC FACTS JSON:
%s

RESUME DATA:
%s

TARGET JOB DESCRIPTION:
%s`, string(factsJSON), resumeText, strings.TrimSpace(jobDescription))
}

func scanExperienceFacts(experiences []map[string]interface{}, facts *ResumeAnalysisFacts) {
	for i, exp := range experiences {
		entryLabel := fmt.Sprintf("experience %d", i+1)
		title := strings.TrimSpace(toAnalysisString(exp["jobTitle"]))
		company := strings.TrimSpace(toAnalysisString(exp["company"]))
		location := firstNonEmptyAnalysisString(exp, "location", "city", "state")
		startDate := firstNonEmptyAnalysisString(exp, "startDate", "startMonth", "startYear")
		endDate := firstNonEmptyAnalysisString(exp, "endDate", "endMonth", "endYear")
		currentlyWorking := analysisBool(exp["currentlyWorking"])
		description := strings.TrimSpace(toAnalysisString(exp["description"]))

		if title == "" && company == "" && description == "" {
			continue
		}
		if title == "" {
			facts.MissingFields = append(facts.MissingFields, analysisIssue("missing_experience_title", "high", "experience", "Experience entry is missing a job title.", entryLabel, "Add the formal title for this role."))
		}
		if company == "" {
			facts.MissingFields = append(facts.MissingFields, analysisIssue("missing_experience_company", "high", "experience", "Experience entry is missing a company name.", entryLabel, "Add the employer or organization name."))
		}
		if location == "" {
			facts.MissingFields = append(facts.MissingFields, analysisIssue("missing_experience_location", "medium", "experience", "Experience entry is missing location.", entryLabel, "Add city/state, remote, or hybrid location."))
		}
		if startDate == "" || (endDate == "" && !currentlyWorking) {
			facts.Counts.MissingExperienceDates++
			facts.MissingFields = append(facts.MissingFields, analysisIssue("missing_experience_dates", "high", "experience", "Experience entry is missing start/end dates.", entryLabel, "Add start and end dates, or mark the role as current."))
		}
		if description == "" {
			facts.MissingFields = append(facts.MissingFields, analysisIssue("missing_experience_bullets", "high", "experience", "Experience entry is missing achievement bullets.", entryLabel, "Add 3-5 achievement bullets with action, scope, and result."))
			continue
		}

		for _, bullet := range splitAnalysisBullets(description) {
			facts.Counts.BulletCount++
			if resumeMetricPattern.MatchString(bullet) {
				facts.Counts.BulletsWithMetrics++
				if len(facts.StrengthSignals) < 8 {
					facts.StrengthSignals = append(facts.StrengthSignals, analysisSignal("quantified_achievement", "experience", "Bullet includes quantified evidence.", truncateAnalysisText(bullet, 180)))
				}
			} else {
				facts.Counts.BulletsWithoutMetrics++
			}
			if weakBulletPattern.MatchString(bullet) {
				facts.Counts.WeakVerbBulletCount++
				facts.QualityIssues = append(facts.QualityIssues, analysisIssue("weak_bullet_verb", "medium", "experience", "Bullet starts with a weak or passive verb.", truncateAnalysisText(bullet, 160), "Rewrite with a stronger action verb and concrete outcome."))
			}
			if len(strings.Fields(bullet)) > 35 {
				facts.Counts.LongBulletCount++
				facts.QualityIssues = append(facts.QualityIssues, analysisIssue("long_bullet", "low", "experience", "Bullet is likely too long for recruiter scanning.", truncateAnalysisText(bullet, 180), "Split or tighten the bullet to one clear action and result."))
			}
		}
	}

	if facts.Counts.BulletCount > 0 && facts.Counts.BulletsWithoutMetrics > facts.Counts.BulletsWithMetrics {
		facts.QualityIssues = append(facts.QualityIssues, analysisIssue("low_metric_density", "medium", "experience", "Most experience bullets do not include metrics or scale.", fmt.Sprintf("%d of %d bullets lack metrics.", facts.Counts.BulletsWithoutMetrics, facts.Counts.BulletCount), "Add measurable scope, performance, cost, reliability, user, or time outcomes where truthful."))
	}
}

func scanProjectFacts(projects []map[string]interface{}, facts *ResumeAnalysisFacts) {
	for i, project := range projects {
		label := fmt.Sprintf("project %d", i+1)
		name := strings.TrimSpace(toAnalysisString(project["projectName"]))
		description := strings.TrimSpace(toAnalysisString(project["description"]))
		technologies := strings.TrimSpace(toAnalysisString(project["technologies"]))
		if name == "" && description == "" && technologies == "" {
			continue
		}
		if name == "" {
			facts.MissingFields = append(facts.MissingFields, analysisIssue("missing_project_name", "medium", "projects", "Project entry is missing a name.", label, "Add a concise project name."))
		}
		if description == "" {
			facts.MissingFields = append(facts.MissingFields, analysisIssue("missing_project_description", "medium", "projects", "Project entry is missing a description.", label, "Add what you built, technologies used, and impact."))
		}
		if technologies == "" {
			facts.MissingFields = append(facts.MissingFields, analysisIssue("missing_project_technologies", "low", "projects", "Project entry is missing technologies.", label, "List the main languages, frameworks, and tools used."))
		}
	}
}

func buildResumeAnalysisText(resumeData map[string]interface{}) string {
	var b strings.Builder
	writeField := func(label, key string) {
		if value := strings.TrimSpace(toAnalysisString(resumeData[key])); value != "" {
			fmt.Fprintf(&b, "%s: %s\n", label, value)
		}
	}
	writeField("Name", "name")
	writeField("Email", "email")
	writeField("Phone", "phone")
	writeField("Summary", "summary")
	writeField("Skills", "skills")

	for i, exp := range analysisMapSlice(resumeData["experiences"]) {
		if !hasMeaningfulAnalysisEntry([]map[string]interface{}{exp}, "jobTitle", "company", "description") {
			continue
		}
		fmt.Fprintf(&b, "\nExperience %d:\n", i+1)
		writeAnalysisMapField(&b, exp, "Job Title", "jobTitle")
		writeAnalysisMapField(&b, exp, "Company", "company")
		writeAnalysisMapField(&b, exp, "Location", "location", "city", "state")
		writeAnalysisMapField(&b, exp, "Start", "startDate", "startMonth", "startYear")
		writeAnalysisMapField(&b, exp, "End", "endDate", "endMonth", "endYear")
		writeAnalysisMapField(&b, exp, "Description", "description")
	}

	for i, edu := range analysisMapSlice(resumeData["education"]) {
		if !hasMeaningfulAnalysisEntry([]map[string]interface{}{edu}, "degree", "school", "field", "graduationYear") {
			continue
		}
		fmt.Fprintf(&b, "\nEducation %d:\n", i+1)
		writeAnalysisMapField(&b, edu, "Degree", "degree")
		writeAnalysisMapField(&b, edu, "School", "school")
		writeAnalysisMapField(&b, edu, "Field", "field")
		writeAnalysisMapField(&b, edu, "Graduation", "graduationDate", "graduationMonth", "graduationYear")
		writeAnalysisMapField(&b, edu, "GPA", "gpa")
	}

	for i, project := range analysisMapSlice(resumeData["projects"]) {
		if !hasMeaningfulAnalysisEntry([]map[string]interface{}{project}, "projectName", "description", "technologies") {
			continue
		}
		fmt.Fprintf(&b, "\nProject %d:\n", i+1)
		writeAnalysisMapField(&b, project, "Name", "projectName")
		writeAnalysisMapField(&b, project, "Technologies", "technologies")
		writeAnalysisMapField(&b, project, "Description", "description")
	}

	return strings.TrimSpace(b.String())
}

func writeAnalysisMapField(b *strings.Builder, m map[string]interface{}, label string, keys ...string) {
	if value := firstNonEmptyAnalysisString(m, keys...); value != "" {
		fmt.Fprintf(b, "  %s: %s\n", label, value)
	}
}

func analysisIssue(code, severity, section, message, evidence, action string) ResumeAnalysisIssue {
	return ResumeAnalysisIssue{Code: code, Severity: severity, Section: section, Message: message, Evidence: evidence, Action: action}
}

func analysisSignal(code, section, message, evidence string) ResumeAnalysisSignal {
	return ResumeAnalysisSignal{Code: code, Section: section, Message: message, Evidence: evidence}
}

func sortAnalysisFacts(facts *ResumeAnalysisFacts) {
	sort.SliceStable(facts.MissingFields, func(i, j int) bool {
		return analysisSeverityRank(facts.MissingFields[i].Severity) > analysisSeverityRank(facts.MissingFields[j].Severity)
	})
	sort.SliceStable(facts.QualityIssues, func(i, j int) bool {
		return analysisSeverityRank(facts.QualityIssues[i].Severity) > analysisSeverityRank(facts.QualityIssues[j].Severity)
	})
}

func analysisSeverityRank(severity string) int {
	switch strings.ToLower(severity) {
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func analysisMapSlice(value interface{}) []map[string]interface{} {
	switch typed := value.(type) {
	case []map[string]interface{}:
		return typed
	case []interface{}:
		result := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			if m, ok := item.(map[string]interface{}); ok {
				result = append(result, m)
			}
		}
		return result
	default:
		return nil
	}
}

func hasMeaningfulAnalysisEntry(entries []map[string]interface{}, keys ...string) bool {
	return countMeaningfulAnalysisEntries(entries, keys...) > 0
}

func countMeaningfulAnalysisEntries(entries []map[string]interface{}, keys ...string) int {
	count := 0
	for _, entry := range entries {
		if firstNonEmptyAnalysisString(entry, keys...) != "" {
			count++
		}
	}
	return count
}

func firstNonEmptyAnalysisString(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(toAnalysisString(m[key])); value != "" {
			return value
		}
	}
	return ""
}

func toAnalysisString(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []string:
		return strings.Join(typed, ", ")
	case []interface{}:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(toAnalysisString(item)); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, ", ")
	case int:
		return fmt.Sprintf("%d", typed)
	case int64:
		return fmt.Sprintf("%d", typed)
	case float64:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", typed), "0"), ".")
	default:
		return ""
	}
}

func splitAnalysisList(value interface{}) []string {
	raw := toAnalysisString(value)
	if raw == "" {
		return nil
	}
	parts := regexp.MustCompile(`[,;\n]+`).Split(raw, -1)
	result := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		item := strings.TrimSpace(part)
		key := strings.ToLower(item)
		if item != "" && !seen[key] {
			seen[key] = true
			result = append(result, item)
		}
	}
	return result
}

func splitAnalysisBullets(description string) []string {
	lines := regexp.MustCompile(`\r?\n+`).Split(description, -1)
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(strings.TrimLeft(line, "-* "))
		if line != "" {
			result = append(result, line)
		}
	}
	if len(result) == 0 && strings.TrimSpace(description) != "" {
		return []string{strings.TrimSpace(description)}
	}
	return result
}

func analysisBool(value interface{}) bool {
	if b, ok := value.(bool); ok {
		return b
	}
	if s, ok := value.(string); ok {
		return strings.EqualFold(strings.TrimSpace(s), "true") || strings.EqualFold(strings.TrimSpace(s), "present") || strings.EqualFold(strings.TrimSpace(s), "current")
	}
	return false
}

func truncateAnalysisText(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	return strings.TrimSpace(value[:max]) + "..."
}
