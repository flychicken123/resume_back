package services

import (
	"strings"

	"resumeai/models"
)

type jobCareerFieldRule struct {
	field    CareerField
	keywords []string
}

var jobCareerFieldFallbackRules = []jobCareerFieldRule{
	{
		field: CareerFieldHRRecruiting,
		keywords: []string{
			"recruiter", "recruiting", "recruitment", "talent acquisition", "human resources",
			"people operations", "hr business partner", "hrbp",
		},
	},
	{
		field: CareerFieldDataScience,
		keywords: []string{
			"data scientist", "data science", "machine learning", "ml engineer", "ai research",
			"research scientist", "genai", "generative ai", "artificial intelligence", "analytics",
			"business intelligence", "data engineer", "data analyst",
		},
	},
	{
		field: CareerFieldSoftwareEngineering,
		keywords: []string{
			"software", "developer", "frontend", "front end", "backend", "back end", "full stack",
			"site reliability", "sre", "devops", "platform engineer", "cloud engineer",
			"security engineer", "cybersecurity", "information security", "firmware",
			"embedded software", "rf software", "mobile engineer", "web engineer",
		},
	},
	{
		field: CareerFieldProductManagement,
		keywords: []string{
			"product manager", "product management", "product owner", "program manager",
			"technical program manager",
		},
	},
	{
		field: CareerFieldDesign,
		keywords: []string{
			"designer", "ux", "ui", "user experience", "user interface", "product design",
			"visual design", "graphic design",
		},
	},
	{
		field: CareerFieldSales,
		keywords: []string{
			"sales", "account executive", "account manager", "business development",
			"sales engineer", "solutions consultant", "revenue",
		},
	},
	{
		field: CareerFieldMarketing,
		keywords: []string{
			"marketing", "growth", "brand", "content", "communications", "social media",
			"seo", "demand generation",
		},
	},
	{
		field: CareerFieldFinance,
		keywords: []string{
			"finance", "financial", "accounting", "accountant", "controller", "controlling",
			"tax", "treasury", "fp&a", "auditor", "payroll",
		},
	},
	{
		field: CareerFieldCustomerSuccess,
		keywords: []string{
			"customer success", "customer support", "technical support", "client success",
			"implementation consultant", "support engineer", "customer experience",
		},
	},
	{
		field: CareerFieldOperations,
		keywords: []string{
			"operations", "supply chain", "logistics", "manufacturing", "quality",
			"supplier quality", "facilities", "plant", "technician", "mechanic", "warehouse",
			"procurement", "purchasing", "production", "maintenance", "inventory",
			"industrial", "civil engineer", "structural engineer", "automation controls",
			"water treatment", "safety", "environmental", "construction", "land development",
		},
	},
}

func fallbackJobClassification(job *models.JobPosting) (CareerField, []string, string) {
	return finalizeJobClassification(job, CareerFieldUnknown, nil, "")
}

func finalizeJobClassification(job *models.JobPosting, field CareerField, skills []string, seniority string) (CareerField, []string, string) {
	skills = enrichJobClassificationSkills(job, skills)

	fallbackField := inferJobCareerField(job)
	if existingField := existingJobCareerField(job); existingField != CareerFieldUnknown && existingField != CareerFieldOther {
		field = existingField
	} else if field == CareerFieldUnknown || (field == CareerFieldOther && fallbackField != CareerFieldUnknown && fallbackField != CareerFieldOther) {
		field = fallbackField
	}
	if field == CareerFieldUnknown {
		field = CareerFieldOther
	}

	fallbackSeniority := inferJobSeniority(job)
	if existingSeniority := existingJobSeniority(job); existingSeniority != "" {
		seniority = existingSeniority
	} else {
		seniority = normalizeJobSeniority(seniority)
		if seniority == "" || (seniority == "mid" && fallbackSeniority != "" && fallbackSeniority != "mid") {
			seniority = fallbackSeniority
		}
	}
	if seniority == "" {
		seniority = "mid"
	}

	return field, skills, seniority
}

func existingJobCareerField(job *models.JobPosting) CareerField {
	if job == nil || strings.TrimSpace(job.CareerField) == "" {
		return CareerFieldUnknown
	}
	return normalizeJobCareerField(job.CareerField)
}

func existingJobSeniority(job *models.JobPosting) string {
	if job == nil {
		return ""
	}
	return normalizeJobSeniority(job.Seniority)
}

func normalizeJobSeniority(value string) string {
	seniority := strings.ToLower(strings.TrimSpace(value))
	if validSeniorityValues[seniority] {
		return seniority
	}
	return ""
}

func inferJobSeniority(job *models.JobPosting) string {
	if job == nil {
		return "mid"
	}
	return determineJobSeniority(job.Title, strings.Join([]string{job.Department, job.Description}, " "))
}

func inferJobCareerField(job *models.JobPosting) CareerField {
	if job == nil {
		return CareerFieldOther
	}
	text := normalizeJobClassificationSearchText(strings.Join([]string{job.Title, job.Department, job.Description}, " "))
	if text == "" {
		return CareerFieldOther
	}
	for _, rule := range jobCareerFieldFallbackRules {
		for _, keyword := range rule.keywords {
			if containsJobClassificationKeyword(text, keyword) {
				return rule.field
			}
		}
	}
	return CareerFieldOther
}

func containsJobClassificationKeyword(text, keyword string) bool {
	keyword = normalizeJobClassificationSearchText(keyword)
	if keyword == "" {
		return false
	}
	return strings.Contains(" "+text+" ", " "+keyword+" ")
}

func normalizeJobClassificationSearchText(value string) string {
	value = strings.ToLower(strings.ToValidUTF8(value, ""))
	replacer := strings.NewReplacer(
		"\r", " ", "\n", " ", "\t", " ",
		".", " ", ",", " ", ";", " ", ":", " ",
		"(", " ", ")", " ", "[", " ", "]", " ",
		"{", " ", "}", " ", "-", " ", "_", " ",
		"/", " ", "\\", " ", "|", " ", "&", " ",
	)
	return strings.Join(strings.Fields(replacer.Replace(value)), " ")
}
