package web

import (
	"context"
	"fmt"
	"time"
)

// ClientConfig holds configuration for the web client.
type ClientConfig struct {
	BraveAPIKey string
	IdleTimeout time.Duration
}

// Client provides a unified interface for web navigation and content extraction.
type Client struct {
	pool       *BrowserPool
	extractor  *ContentExtractor
	searcher   *WebSearcher
	actions    *Actions
	screenshot *Screenshot
}

// NewClient creates a new web client with default settings.
func NewClient() *Client {
	return NewClientWithConfig(ClientConfig{})
}

// NewClientWithConfig creates a new web client with custom configuration.
func NewClientWithConfig(cfg ClientConfig) *Client {
	idleTimeout := cfg.IdleTimeout
	if idleTimeout == 0 {
		idleTimeout = 5 * time.Minute
	}

	pool := NewBrowserPool(idleTimeout)

	return &Client{
		pool:       pool,
		extractor:  NewContentExtractor(),
		searcher:   NewWebSearcherWithConfig(pool, cfg.BraveAPIKey),
		actions:    NewActions(pool),
		screenshot: NewScreenshot(pool),
	}
}

// NewClientWithTimeout creates a new web client with a custom idle timeout.
func NewClientWithTimeout(idleTimeout time.Duration) *Client {
	return NewClientWithConfig(ClientConfig{IdleTimeout: idleTimeout})
}

// Close closes the client and releases resources.
func (c *Client) Close() {
	c.pool.Close()
}

// SetVisible sets the browser visibility mode.
func (c *Client) SetVisible(visible bool) error {
	return c.pool.SetVisible(visible)
}

// IsVisible returns whether the browser is in visible mode.
func (c *Client) IsVisible() bool {
	return c.pool.IsVisible()
}

// IsRunning returns whether the browser is currently running.
func (c *Client) IsRunning() bool {
	return c.pool.IsRunning()
}

// --- Navigation ---

// Navigate navigates to a URL and waits for the page to load.
func (c *Client) Navigate(url string) error {
	return c.pool.Navigate(url, 60*time.Second)
}

// NavigateWithTimeout navigates to a URL with a custom timeout.
func (c *Client) NavigateWithTimeout(url string, timeout time.Duration) error {
	return c.pool.Navigate(url, timeout)
}

// NavigateAndWait navigates to a URL and waits for a specific element.
func (c *Client) NavigateAndWait(url string, waitSelector string, timeout time.Duration) error {
	if err := c.pool.Navigate(url, timeout); err != nil {
		return err
	}

	if waitSelector != "" {
		return c.pool.WaitFor(waitSelector, timeout)
	}

	return nil
}

// GetCurrentURL returns the current page URL.
func (c *Client) GetCurrentURL() (string, error) {
	return c.pool.GetCurrentURL()
}

// GetTitle returns the current page title.
func (c *Client) GetTitle() (string, error) {
	return c.pool.GetTitle()
}

// --- Content Extraction ---

// ReadContent extracts the main content from the current page.
func (c *Client) ReadContent() (*PageContent, error) {
	html, err := c.pool.GetHTML()
	if err != nil {
		return nil, fmt.Errorf("failed to get HTML: %w", err)
	}

	url, err := c.pool.GetCurrentURL()
	if err != nil {
		return nil, fmt.Errorf("failed to get URL: %w", err)
	}

	return c.extractor.ExtractContent(html, url)
}

// ReadContentFromSelector extracts content from a specific selector.
func (c *Client) ReadContentFromSelector(selector string) (string, error) {
	html, err := c.pool.GetHTML()
	if err != nil {
		return "", fmt.Errorf("failed to get HTML: %w", err)
	}

	return c.extractor.ExtractText(html, selector)
}

// ExtractLinks extracts all links from the current page.
func (c *Client) ExtractLinks() ([]Link, error) {
	html, err := c.pool.GetHTML()
	if err != nil {
		return nil, fmt.Errorf("failed to get HTML: %w", err)
	}

	url, err := c.pool.GetCurrentURL()
	if err != nil {
		return nil, fmt.Errorf("failed to get URL: %w", err)
	}

	return c.extractor.ExtractLinks(html, url)
}

// GetPageInfo returns basic information about the current page.
func (c *Client) GetPageInfo() (*PageInfo, error) {
	html, err := c.pool.GetHTML()
	if err != nil {
		return nil, fmt.Errorf("failed to get HTML: %w", err)
	}

	url, err := c.pool.GetCurrentURL()
	if err != nil {
		return nil, fmt.Errorf("failed to get URL: %w", err)
	}

	return c.extractor.GetPageInfo(html, url)
}

// GetMetaContent extracts content from a meta tag.
func (c *Client) GetMetaContent(name string) (string, error) {
	html, err := c.pool.GetHTML()
	if err != nil {
		return "", fmt.Errorf("failed to get HTML: %w", err)
	}

	return c.extractor.GetMetaContent(html, name)
}

// --- Search ---

// Search performs a web search.
func (c *Client) Search(ctx context.Context, query string, maxResults int) (*SearchResults, error) {
	return c.searcher.Search(ctx, query, maxResults)
}

// --- Actions ---

// Click clicks on an element.
func (c *Client) Click(selector string) error {
	return c.actions.Click(selector, true)
}

// ClickNoWait clicks on an element without waiting for navigation.
func (c *Client) ClickNoWait(selector string) error {
	return c.actions.Click(selector, false)
}

// ClickByText clicks on an element containing the specified text.
func (c *Client) ClickByText(text string) error {
	return c.actions.ClickByText(text, true)
}

// Type types text into an input field.
func (c *Client) Type(selector string, text string) error {
	return c.actions.Type(selector, text, true, false)
}

// TypeAndSubmit types text and presses Enter.
func (c *Client) TypeAndSubmit(selector string, text string) error {
	return c.actions.Type(selector, text, true, true)
}

// Scroll scrolls the page.
func (c *Client) Scroll(direction ScrollDirection, amount int) error {
	return c.actions.Scroll(direction, amount)
}

// ScrollToElement scrolls an element into view.
func (c *Client) ScrollToElement(selector string) error {
	return c.actions.ScrollToElement(selector)
}

// Wait waits for a condition on an element.
func (c *Client) Wait(selector string, condition WaitCondition, timeout time.Duration) error {
	return c.actions.Wait(selector, condition, timeout)
}

// WaitVisible waits for an element to be visible.
func (c *Client) WaitVisible(selector string) error {
	return c.actions.Wait(selector, WaitVisible, 10*time.Second)
}

// ElementExists checks if an element exists.
func (c *Client) ElementExists(selector string) (bool, error) {
	return c.actions.ElementExists(selector)
}

// GetElementText gets the text content of an element.
func (c *Client) GetElementText(selector string) (string, error) {
	return c.actions.GetElementText(selector)
}

// --- Screenshot ---

// CaptureScreenshot captures a screenshot of the current viewport.
func (c *Client) CaptureScreenshot() ([]byte, error) {
	return c.screenshot.CaptureViewport()
}

// CaptureFullPageScreenshot captures a screenshot of the full page.
func (c *Client) CaptureFullPageScreenshot() ([]byte, error) {
	return c.screenshot.CaptureFullPage()
}

// CaptureElementScreenshot captures a screenshot of a specific element.
func (c *Client) CaptureElementScreenshot(selector string) ([]byte, error) {
	return c.screenshot.CaptureElement(selector)
}

// --- Login/Visible Mode ---

// RequestLogin opens the browser in visible mode for manual authentication.
func (c *Client) RequestLogin(loginURL string) error {
	// Switch to visible mode
	if err := c.SetVisible(true); err != nil {
		return fmt.Errorf("failed to set visible mode: %w", err)
	}

	// Navigate to login page
	if err := c.Navigate(loginURL); err != nil {
		return fmt.Errorf("failed to navigate to login page: %w", err)
	}

	return nil
}

// FinishLogin switches back to headless mode after login.
// Call this after the user has completed authentication.
func (c *Client) FinishLogin() error {
	// Note: We don't switch back to headless immediately
	// because that would close the browser and lose the session.
	// The browser will stay visible until the next operation
	// that requires headless mode, or the idle timeout is reached.
	return nil
}

// SwitchToHeadless switches to headless mode.
func (c *Client) SwitchToHeadless() error {
	return c.SetVisible(false)
}

// --- Helper Methods ---

// Sleep pauses for the specified duration.
func (c *Client) Sleep(duration time.Duration) error {
	return c.pool.Sleep(duration)
}

// GetHTML returns the full HTML of the current page.
func (c *Client) GetHTML() (string, error) {
	return c.pool.GetHTML()
}
