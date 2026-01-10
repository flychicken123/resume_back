package services

import (
	"sort"
	"strings"
)

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

// skillInferenceRules maps implicit skills to the explicit skills that imply them.
// If a user has ANY of the listed skills, they implicitly have the key skill.
var skillInferenceRules = map[string][]string{
	// Frontend inference
	"frontend": {
		"react", "vue", "angular", "svelte", "next", "nuxt",
		"html", "css", "sass", "less", "tailwind", "bootstrap",
		"webpack", "vite", "rollup", "parcel",
	},

	// Backend inference
	"backend": {
		"node", "express", "django", "flask", "fastapi",
		"spring", "springboot", "rails", "laravel", "symfony",
		"gin", "echo", "fiber", "actix", "rocket",
		"nestjs", "koa", "hapi",
	},

	// Database inference
	"database": {
		"postgresql", "mysql", "mongodb", "redis", "elasticsearch",
		"cassandra", "sqlite", "oracle", "sql server", "dynamodb",
		"firestore", "supabase", "prisma", "sequelize", "typeorm",
	},

	// DevOps inference
	"devops": {
		"docker", "kubernetes", "terraform", "ansible", "puppet", "chef",
		"jenkins", "github actions", "gitlab ci", "circleci", "travis",
		"prometheus", "grafana", "datadog", "splunk", "elk",
	},

	// Cloud inference
	"cloud": {
		"aws", "gcp", "azure", "heroku", "vercel", "netlify",
		"digitalocean", "linode", "cloudflare",
	},

	// API inference
	"api": {
		"rest", "restful", "graphql", "grpc", "soap",
		"openapi", "swagger", "postman",
	},

	// Machine Learning inference
	"machine learning": {
		"tensorflow", "pytorch", "keras", "scikit-learn", "sklearn",
		"pandas", "numpy", "jupyter", "mlflow", "huggingface",
	},

	// Mobile inference
	"mobile": {
		"react native", "flutter", "swift", "kotlin", "ios", "android",
		"xamarin", "ionic", "capacitor",
	},

	// Testing inference
	"testing": {
		"jest", "mocha", "pytest", "junit", "rspec",
		"cypress", "playwright", "selenium", "testcafe",
	},

	// CI/CD inference
	"ci/cd": {
		"jenkins", "github actions", "gitlab ci", "circleci", "travis",
		"azure devops", "bamboo", "teamcity", "drone",
	},

	// Microservices inference
	"microservices": {
		"kubernetes", "docker", "kafka", "rabbitmq", "grpc",
		"service mesh", "istio", "envoy", "consul",
	},
}

// InferImplicitSkills takes a list of explicit skills and returns additional
// implicit skills that can be inferred from them.
// For example, if user has ["react", "node", "postgresql"], this returns
// ["frontend", "backend", "database", "fullstack"].
func InferImplicitSkills(skills []string) []string {
	if len(skills) == 0 {
		return nil
	}

	// Normalize input skills to lowercase
	normalizedSkills := make(map[string]bool, len(skills))
	for _, s := range skills {
		canonical := GetCanonicalSkill(s)
		normalizedSkills[canonical] = true
		// Also add the original lowercased version
		normalizedSkills[strings.ToLower(strings.TrimSpace(s))] = true
	}

	inferred := make(map[string]bool)

	// Check each inference rule
	for implicitSkill, triggerSkills := range skillInferenceRules {
		for _, trigger := range triggerSkills {
			if normalizedSkills[trigger] {
				inferred[implicitSkill] = true
				break
			}
		}
	}

	// Special case: if user has both frontend AND backend, infer fullstack
	if inferred["frontend"] && inferred["backend"] {
		inferred["fullstack"] = true
	}

	// Don't include skills user already has
	for skill := range inferred {
		if normalizedSkills[skill] {
			delete(inferred, skill)
		}
	}

	if len(inferred) == 0 {
		return nil
	}

	// Convert to sorted slice
	result := make([]string, 0, len(inferred))
	for skill := range inferred {
		result = append(result, skill)
	}
	sort.Strings(result)
	return result
}

// ExpandSkillsWithInference takes explicit skills and returns both the explicit
// skills (canonicalized) plus any inferred implicit skills.
func ExpandSkillsWithInference(skills []string) []string {
	if len(skills) == 0 {
		return nil
	}

	// Start with canonicalized explicit skills
	result := make(map[string]bool, len(skills)*2)
	for _, s := range skills {
		canonical := GetCanonicalSkill(s)
		result[canonical] = true
	}

	// Add inferred skills
	inferred := InferImplicitSkills(skills)
	for _, s := range inferred {
		result[s] = true
	}

	// Convert to sorted slice
	out := make([]string, 0, len(result))
	for skill := range result {
		out = append(out, skill)
	}
	sort.Strings(out)
	return out
}

// ExtractSkillsFromText scans text for known skills from our synonym map.
// Returns canonical skill names found in the text, sorted alphabetically.
func ExtractSkillsFromText(text string) []string {
	if text == "" {
		return nil
	}

	text = strings.ToLower(text)
	found := make(map[string]bool)

	// Check each canonical skill and its variations
	for canonical, variations := range skillSynonyms {
		// Check canonical form
		if strings.Contains(text, canonical) {
			found[canonical] = true
			continue
		}
		// Check variations
		for _, v := range variations {
			if strings.Contains(text, v) {
				found[canonical] = true
				break
			}
		}
	}

	// Convert to sorted slice
	result := make([]string, 0, len(found))
	for skill := range found {
		result = append(result, skill)
	}
	sort.Strings(result)
	return result
}
