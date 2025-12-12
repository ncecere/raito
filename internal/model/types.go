package model

// Metadata is a trimmed version of Firecrawl's metadata block.
type Metadata struct {
	Title         string   `json:"title,omitempty"`
	Description   string   `json:"description,omitempty"`
	Language      string   `json:"language,omitempty"`
	Keywords      string   `json:"keywords,omitempty"`
	Robots        string   `json:"robots,omitempty"`
	OgTitle       string   `json:"ogTitle,omitempty"`
	OgDescription string   `json:"ogDescription,omitempty"`
	OgURL         string   `json:"ogUrl,omitempty"`
	OgImage       string   `json:"ogImage,omitempty"`
	OgLocaleAlt   []string `json:"ogLocaleAlternate,omitempty"`
	OgSiteName    string   `json:"ogSiteName,omitempty"`
	SourceURL     string   `json:"sourceURL,omitempty"`
	StatusCode    int      `json:"statusCode"`
}

// Document is a reduced version of Firecrawl's Document type
// sufficient for scrape/map/crawl responses.
type Document struct {
	Markdown string   `json:"markdown,omitempty"`
	HTML     string   `json:"html,omitempty"`
	RawHTML  string   `json:"rawHtml,omitempty"`
	Links    []string `json:"links,omitempty"`
	Images   []string `json:"images,omitempty"`
	Engine   string   `json:"engine,omitempty"`
	Metadata Metadata `json:"metadata"`
}
