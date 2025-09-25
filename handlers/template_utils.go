package handlers

import "strings"

const defaultTemplateSlug = "classic-professional"

var (
	templateSlugAliases = map[string]string{
		"temp1":            defaultTemplateSlug,
		"modern":           "modern-clean",
		"industry-manager": "executive-serif",
	}

	validTemplateSlugs = map[string]struct{}{
		defaultTemplateSlug: {},
		"modern-clean":      {},
		"executive-serif":   {},
	}
)

func normalizeTemplateFormat(value string) string {
	slug := strings.TrimSpace(strings.ToLower(value))
	if slug == "" {
		return defaultTemplateSlug
	}

	if mapped, ok := templateSlugAliases[slug]; ok {
		slug = mapped
	}

	if _, ok := validTemplateSlugs[slug]; ok {
		return slug
	}

	return defaultTemplateSlug
}
