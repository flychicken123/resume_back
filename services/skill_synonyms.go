package services

import "strings"

// skillSynonyms maps canonical skill names to their common variations.
// This enables matching "react" to find jobs mentioning "reactjs", "react.js", etc.
var skillSynonyms = map[string][]string{
	// JavaScript ecosystem
	"javascript":  {"js", "ecmascript", "es6", "es2015", "es2016", "es2017"},
	"typescript":  {"ts"},
	"react":       {"reactjs", "react.js", "react native", "reactnative"},
	"vue":         {"vuejs", "vue.js", "vue 3", "vue3"},
	"angular":     {"angularjs", "angular.js", "angular 2"},
	"node":        {"nodejs", "node.js"},
	"next":        {"nextjs", "next.js"},
	"nuxt":        {"nuxtjs", "nuxt.js"},
	"express":     {"expressjs", "express.js"},

	// Backend languages
	"python":  {"py", "python3", "python 3"},
	"golang":  {"go"},
	"java":    {"jdk", "jvm", "openjdk"},
	"csharp":  {"c#", ".net", "dotnet"},
	"ruby":    {"rails", "ruby on rails", "ror"},
	"php":     {"laravel", "symfony"},
	"rust":    {"rustlang"},
	"scala":   {"akka", "play framework"},
	"kotlin":  {"android", "jetpack"},
	"swift":   {"ios", "swiftui", "uikit"},

	// DevOps & Infrastructure
	"kubernetes": {"k8s", "kube", "eks", "gke", "aks"},
	"docker":     {"containers", "containerization", "dockerfile"},
	"terraform":  {"tf", "iac", "infrastructure as code"},
	"ansible":    {"playbook", "configuration management"},
	"jenkins":    {"ci server", "build server"},
	"github actions": {"gha", "gh actions"},
	"gitlab ci":      {"gitlab pipeline"},

	// Cloud providers
	"aws":   {"amazon web services", "ec2", "s3", "lambda", "dynamodb", "rds", "cloudfront"},
	"gcp":   {"google cloud", "google cloud platform", "bigquery", "cloud run"},
	"azure": {"microsoft azure", "azure devops"},

	// Databases
	"postgresql": {"postgres", "psql", "pg"},
	"mongodb":    {"mongo", "nosql"},
	"mysql":      {"mariadb"},
	"redis":      {"elasticache", "memcached"},
	"elasticsearch": {"es", "elastic", "opensearch", "elk"},
	"cassandra":     {"scylla", "dynamodb"},
	"sqlite":        {"sql lite"},

	// AI/ML
	"machine learning":        {"ml", "deep learning", "neural networks", "neural network"},
	"artificial intelligence": {"ai", "genai", "generative ai", "llm", "large language model"},
	"tensorflow":              {"tf", "keras"},
	"pytorch":                 {"torch"},
	"data science":            {"data scientist", "analytics", "data analysis"},
	"nlp":                     {"natural language processing", "text processing"},
	"computer vision":         {"cv", "image recognition", "object detection"},

	// Data Engineering
	"apache spark": {"spark", "pyspark"},
	"apache kafka": {"kafka", "event streaming"},
	"airflow":      {"apache airflow", "dag", "workflow orchestration"},
	"snowflake":    {"data warehouse", "dwh"},
	"databricks":   {"lakehouse"},

	// General concepts
	"ci/cd":    {"cicd", "ci cd", "continuous integration", "continuous deployment", "continuous delivery"},
	"devops":   {"sre", "site reliability", "platform engineering"},
	"agile":    {"scrum", "kanban", "sprint"},
	"frontend": {"front end", "front-end", "ui", "user interface"},
	"backend":  {"back end", "back-end", "server side", "server-side"},
	"fullstack": {"full stack", "full-stack"},
	"api":       {"rest", "restful", "graphql", "grpc"},
	"microservices": {"micro services", "micro-services", "distributed systems"},
	"testing":       {"unit testing", "integration testing", "e2e", "tdd", "bdd"},

	// Security
	"security":   {"cybersecurity", "infosec", "appsec", "devsecops"},
	"oauth":      {"oauth2", "oidc", "openid"},
	"encryption": {"cryptography", "ssl", "tls"},
}

// skillToCanonical maps all variations back to their canonical form.
var skillToCanonical map[string]string

func init() {
	skillToCanonical = make(map[string]string, len(skillSynonyms)*5)
	for canonical, variations := range skillSynonyms {
		skillToCanonical[canonical] = canonical
		for _, v := range variations {
			skillToCanonical[v] = canonical
		}
	}
}

// ExpandSkillWithSynonyms returns all known variations of a skill.
// Given "react", it returns ["react", "reactjs", "react.js", "react native", "reactnative"].
// For unknown skills, it returns just the original skill.
func ExpandSkillWithSynonyms(skill string) []string {
	skill = strings.ToLower(strings.TrimSpace(skill))
	if skill == "" {
		return nil
	}

	// Check if skill is a known variation or canonical form
	if canonical, ok := skillToCanonical[skill]; ok {
		result := make([]string, 0, 6)
		result = append(result, skill)

		// Add canonical if different from input
		if canonical != skill {
			result = append(result, canonical)
		}

		// Add all variations
		if variations, ok := skillSynonyms[canonical]; ok {
			for _, v := range variations {
				if v != skill {
					result = append(result, v)
				}
			}
		}
		return result
	}

	// Unknown skill - return as-is
	return []string{skill}
}

// GetCanonicalSkill returns the canonical form of a skill, or the skill itself if unknown.
func GetCanonicalSkill(skill string) string {
	skill = strings.ToLower(strings.TrimSpace(skill))
	if canonical, ok := skillToCanonical[skill]; ok {
		return canonical
	}
	return skill
}
