package web

// Link represents a hyperlink extracted from a page.
type Link struct {
	URL      string `json:"url"`
	Text     string `json:"text"`
	Title    string `json:"title,omitempty"`
	External bool   `json:"external"`
}

// PageContent represents extracted content from a web page.
type PageContent struct {
	URL                string `json:"url"`
	Title              string `json:"title"`
	Content            string `json:"content"`
	Excerpt            string `json:"excerpt,omitempty"`
	WordCount          int    `json:"word_count"`
	ReadingTimeMinutes int    `json:"reading_time_minutes,omitempty"`
	Links              []Link `json:"links,omitempty"`
}

// PageInfo represents basic information about the current page.
type PageInfo struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

// ClickOptions configures click behavior.
type ClickOptions struct {
	Selector       string `json:"selector,omitempty"`
	Text           string `json:"text,omitempty"`
	WaitNavigation bool   `json:"wait_navigation"`
}

// TypeOptions configures text input behavior.
type TypeOptions struct {
	Selector   string `json:"selector"`
	Text       string `json:"text"`
	ClearFirst bool   `json:"clear_first"`
	Submit     bool   `json:"submit"`
}

// ScrollOptions configures scroll behavior.
type ScrollOptions struct {
	Direction string `json:"direction,omitempty"` // up, down, top, bottom
	Selector  string `json:"selector,omitempty"`  // scroll to element
	Amount    int    `json:"amount,omitempty"`    // pixels
}

// WaitOptions configures wait behavior.
type WaitOptions struct {
	Selector  string `json:"selector"`
	Condition string `json:"condition"` // visible, hidden, exists, not_exists
	Timeout   int    `json:"timeout"`   // seconds
}

// InspectResult represents the result of page inspection.
type InspectResult struct {
	Screenshot []byte       `json:"screenshot,omitempty"`
	Element    *ElementInfo `json:"element,omitempty"`
	Analysis   string       `json:"analysis,omitempty"`
}

// ElementInfo represents information about a DOM element.
type ElementInfo struct {
	TagName    string            `json:"tag_name"`
	ID         string            `json:"id,omitempty"`
	Classes    []string          `json:"classes,omitempty"`
	Text       string            `json:"text,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
	HTML       string            `json:"html,omitempty"`
}

// NavigateOptions configures navigation behavior.
type NavigateOptions struct {
	URL     string `json:"url"`
	WaitFor string `json:"wait_for,omitempty"`
	Timeout int    `json:"timeout,omitempty"` // seconds
}

// ReadOptions configures content reading behavior.
type ReadOptions struct {
	Selector     string `json:"selector,omitempty"`
	IncludeLinks bool   `json:"include_links"`
	MaxLength    int    `json:"max_length,omitempty"`
}
