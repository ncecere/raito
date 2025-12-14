package http

import "strings"

// getScreenshotFormatConfig scans formats for a screenshot entry and returns
// whether it was requested along with a fullPage flag. It supports both simple
// string formats ("screenshot") and object formats ({type: "screenshot", ...}).
func getScreenshotFormatConfig(formats []interface{}) (bool, bool) {
	if len(formats) == 0 {
		return false, false
	}

	for _, f := range formats {
		switch v := f.(type) {
		case string:
			if strings.ToLower(v) == "screenshot" {
				// Default to full-page screenshots for better parity with Firecrawl.
				return true, true
			}
		case map[string]interface{}:
			rawType, ok := v["type"].(string)
			if !ok || strings.ToLower(rawType) != "screenshot" {
				continue
			}

			fullPage := true
			if fp, ok := v["fullPage"].(bool); ok {
				fullPage = fp
			}

			return true, fullPage
		}
	}

	return false, false
}
