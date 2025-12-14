package scrapeutil

import (
	"net/url"
	"strings"
)

// ToString safely converts an interface value to string.
func ToString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// FilterLinks applies basic link filters based on configuration.
// sameDomainOnly restricts links to those matching the base URL's host.
// maxPerDocument > 0 limits the number of links returned.
func FilterLinks(links []string, baseURL string, sameDomainOnly bool, maxPerDocument int) []string {
	if len(links) == 0 {
		return links
	}

	filtered := make([]string, 0, len(links))

	var baseHost string
	if sameDomainOnly {
		if u, err := url.Parse(baseURL); err == nil {
			baseHost = strings.ToLower(u.Hostname())
		} else {
			// If base URL is invalid, skip same-domain filtering but still apply maxPerDocument.
			sameDomainOnly = false
		}
	}

	for _, link := range links {
		if link == "" {
			continue
		}

		if sameDomainOnly {
			lu, err := url.Parse(link)
			if err != nil {
				continue
			}
			if strings.ToLower(lu.Hostname()) != baseHost {
				continue
			}
		}

		filtered = append(filtered, link)
		if maxPerDocument > 0 && len(filtered) >= maxPerDocument {
			break
		}
	}

	return filtered
}

// WantsFormat inspects a Firecrawl-style formats array to determine
// whether a given format type (e.g., "summary") was requested.
func WantsFormat(formats []interface{}, name string) bool {
	if len(formats) == 0 {
		return false
	}

	target := strings.ToLower(name)
	for _, f := range formats {
		switch v := f.(type) {
		case string:
			if strings.ToLower(v) == target {
				return true
			}
		case map[string]interface{}:
			if t, ok := v["type"]; ok {
				if s, ok := t.(string); ok && strings.ToLower(s) == target {
					return true
				}
			}
		}
	}

	return false
}

// GetJSONFormatConfig scans a Firecrawl-style formats array and returns
// the first json format configuration, if present. It supports both
// simple string formats ("json") and object formats ({type: "json", ...}).
func GetJSONFormatConfig(formats []interface{}) (bool, string, map[string]interface{}) {
	if len(formats) == 0 {
		return false, "", nil
	}

	for _, f := range formats {
		switch v := f.(type) {
		case string:
			if strings.ToLower(v) == "json" {
				return true, "", nil
			}
		case map[string]interface{}:
			rawType, ok := v["type"].(string)
			if !ok || strings.ToLower(rawType) != "json" {
				continue
			}

			prompt := ""
			if p, ok := v["prompt"].(string); ok {
				prompt = p
			}

			var schema map[string]interface{}
			if s, ok := v["schema"].(map[string]interface{}); ok {
				schema = s
			}

			return true, prompt, schema
		}
	}

	return false, "", nil
}

// GetBrandingFormatConfig scans formats for a branding entry and returns
// whether it was requested along with an optional custom prompt.
func GetBrandingFormatConfig(formats []interface{}) (bool, string) {
	if len(formats) == 0 {
		return false, ""
	}

	for _, f := range formats {
		switch v := f.(type) {
		case string:
			if strings.ToLower(v) == "branding" {
				return true, ""
			}
		case map[string]interface{}:
			rawType, ok := v["type"].(string)
			if !ok || strings.ToLower(rawType) != "branding" {
				continue
			}

			prompt := ""
			if p, ok := v["prompt"].(string); ok {
				prompt = p
			}

			return true, prompt
		}
	}

	return false, ""
}

// NormalizeBrandingImages prunes nil values from the images sub-object
// of a branding profile so that fields like favicon and ogImage are
// omitted rather than returned as explicit nulls.
func NormalizeBrandingImages(branding map[string]interface{}) {
	if branding == nil {
		return
	}

	imagesVal, ok := branding["images"]
	if !ok {
		return
	}

	imagesMap, ok := imagesVal.(map[string]interface{})
	if !ok {
		return
	}

	for k, v := range imagesMap {
		if v == nil {
			delete(imagesMap, k)
		}
	}

	if len(imagesMap) == 0 {
		delete(branding, "images")
	} else {
		branding["images"] = imagesMap
	}
}
