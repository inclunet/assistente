package web

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
)

// Actions provides methods for interacting with web pages.
type Actions struct {
	pool *BrowserPool
}

// NewActions creates a new Actions instance.
func NewActions(pool *BrowserPool) *Actions {
	return &Actions{pool: pool}
}

// Click clicks on an element identified by CSS selector.
func (a *Actions) Click(selector string, waitNavigation bool) error {
	browserCtx, err := a.pool.GetContext()
	if err != nil {
		return err
	}

	a.pool.BeginOperation()
	defer a.pool.EndOperation()

	opCtx, cancel := context.WithTimeout(browserCtx, 30*time.Second)
	defer cancel()

	actions := []chromedp.Action{
		chromedp.WaitVisible(selector),
		chromedp.Click(selector),
	}

	if waitNavigation {
		actions = append(actions, chromedp.WaitReady("body"))
	}

	return chromedp.Run(opCtx, actions...)
}

// ClickByText clicks on an element containing the specified text.
func (a *Actions) ClickByText(text string, waitNavigation bool) error {
	browserCtx, err := a.pool.GetContext()
	if err != nil {
		return err
	}

	a.pool.BeginOperation()
	defer a.pool.EndOperation()

	opCtx, cancel := context.WithTimeout(browserCtx, 30*time.Second)
	defer cancel()

	// XPath to find element containing text
	xpath := fmt.Sprintf("//*[contains(text(), '%s')]", text)

	actions := []chromedp.Action{
		chromedp.WaitVisible(xpath),
		chromedp.Click(xpath),
	}

	if waitNavigation {
		actions = append(actions, chromedp.WaitReady("body"))
	}

	return chromedp.Run(opCtx, actions...)
}

// Type types text into an input field.
func (a *Actions) Type(selector string, text string, clearFirst bool, submit bool) error {
	browserCtx, err := a.pool.GetContext()
	if err != nil {
		return err
	}

	a.pool.BeginOperation()
	defer a.pool.EndOperation()

	opCtx, cancel := context.WithTimeout(browserCtx, 30*time.Second)
	defer cancel()

	var actions []chromedp.Action

	// Wait for element
	actions = append(actions, chromedp.WaitVisible(selector))

	// Clear if requested
	if clearFirst {
		actions = append(actions, chromedp.Clear(selector))
	}

	// Type the text
	actions = append(actions, chromedp.SendKeys(selector, text))

	// Submit if requested (press Enter)
	if submit {
		actions = append(actions, chromedp.SendKeys(selector, "\n"))
		actions = append(actions, chromedp.WaitReady("body"))
	}

	return chromedp.Run(opCtx, actions...)
}

// ScrollDirection represents scroll directions.
type ScrollDirection string

const (
	ScrollUp     ScrollDirection = "up"
	ScrollDown   ScrollDirection = "down"
	ScrollTop    ScrollDirection = "top"
	ScrollBottom ScrollDirection = "bottom"
)

// Scroll scrolls the page.
func (a *Actions) Scroll(direction ScrollDirection, amount int) error {
	browserCtx, err := a.pool.GetContext()
	if err != nil {
		return err
	}

	a.pool.BeginOperation()
	defer a.pool.EndOperation()

	opCtx, cancel := context.WithTimeout(browserCtx, 10*time.Second)
	defer cancel()

	if amount <= 0 {
		amount = 500
	}

	var script string
	switch direction {
	case ScrollUp:
		script = fmt.Sprintf("window.scrollBy(0, -%d)", amount)
	case ScrollDown:
		script = fmt.Sprintf("window.scrollBy(0, %d)", amount)
	case ScrollTop:
		script = "window.scrollTo(0, 0)"
	case ScrollBottom:
		script = "window.scrollTo(0, document.body.scrollHeight)"
	default:
		return fmt.Errorf("unknown scroll direction: %s", direction)
	}

	return chromedp.Run(opCtx, chromedp.Evaluate(script, nil))
}

// ScrollToElement scrolls an element into view.
func (a *Actions) ScrollToElement(selector string) error {
	browserCtx, err := a.pool.GetContext()
	if err != nil {
		return err
	}

	a.pool.BeginOperation()
	defer a.pool.EndOperation()

	opCtx, cancel := context.WithTimeout(browserCtx, 10*time.Second)
	defer cancel()

	return chromedp.Run(opCtx,
		chromedp.ScrollIntoView(selector),
	)
}

// WaitCondition represents wait conditions.
type WaitCondition string

const (
	WaitVisible   WaitCondition = "visible"
	WaitHidden    WaitCondition = "hidden"
	WaitExists    WaitCondition = "exists"
	WaitNotExists WaitCondition = "not_exists"
)

// Wait waits for a condition on an element.
func (a *Actions) Wait(selector string, condition WaitCondition, timeout time.Duration) error {
	ctx, err := a.pool.GetContext()
	if err != nil {
		return err
	}

	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var action chromedp.Action
	switch condition {
	case WaitVisible:
		action = chromedp.WaitVisible(selector)
	case WaitHidden:
		action = chromedp.WaitNotVisible(selector)
	case WaitExists:
		action = chromedp.WaitReady(selector)
	case WaitNotExists:
		action = chromedp.WaitNotPresent(selector)
	default:
		return fmt.Errorf("unknown wait condition: %s", condition)
	}

	return chromedp.Run(ctx, action)
}

// Focus focuses on an element.
func (a *Actions) Focus(selector string) error {
	browserCtx, err := a.pool.GetContext()
	if err != nil {
		return err
	}

	a.pool.BeginOperation()
	defer a.pool.EndOperation()

	opCtx, cancel := context.WithTimeout(browserCtx, 10*time.Second)
	defer cancel()

	return chromedp.Run(opCtx,
		chromedp.Focus(selector),
	)
}

// Hover hovers over an element by scrolling it into view.
func (a *Actions) Hover(selector string) error {
	browserCtx, err := a.pool.GetContext()
	if err != nil {
		return err
	}

	a.pool.BeginOperation()
	defer a.pool.EndOperation()

	opCtx, cancel := context.WithTimeout(browserCtx, 10*time.Second)
	defer cancel()

	// Scroll element into view (simulates hover positioning)
	return chromedp.Run(opCtx,
		chromedp.WaitVisible(selector),
		chromedp.ScrollIntoView(selector),
	)
}

// GetElementText gets the text content of an element.
func (a *Actions) GetElementText(selector string) (string, error) {
	browserCtx, err := a.pool.GetContext()
	if err != nil {
		return "", err
	}

	a.pool.BeginOperation()
	defer a.pool.EndOperation()

	opCtx, cancel := context.WithTimeout(browserCtx, 10*time.Second)
	defer cancel()

	var text string
	err = chromedp.Run(opCtx,
		chromedp.Text(selector, &text, chromedp.NodeVisible),
	)

	return text, err
}

// GetElementAttribute gets an attribute value from an element.
func (a *Actions) GetElementAttribute(selector string, attribute string) (string, error) {
	browserCtx, err := a.pool.GetContext()
	if err != nil {
		return "", err
	}

	a.pool.BeginOperation()
	defer a.pool.EndOperation()

	opCtx, cancel := context.WithTimeout(browserCtx, 10*time.Second)
	defer cancel()

	var value string
	err = chromedp.Run(opCtx,
		chromedp.AttributeValue(selector, attribute, &value, nil),
	)

	return value, err
}

// GetElementHTML gets the outer HTML of an element.
func (a *Actions) GetElementHTML(selector string) (string, error) {
	browserCtx, err := a.pool.GetContext()
	if err != nil {
		return "", err
	}

	a.pool.BeginOperation()
	defer a.pool.EndOperation()

	opCtx, cancel := context.WithTimeout(browserCtx, 10*time.Second)
	defer cancel()

	var html string
	err = chromedp.Run(opCtx,
		chromedp.OuterHTML(selector, &html),
	)

	return html, err
}

// ElementExists checks if an element exists.
func (a *Actions) ElementExists(selector string) (bool, error) {
	browserCtx, err := a.pool.GetContext()
	if err != nil {
		return false, err
	}

	a.pool.BeginOperation()
	defer a.pool.EndOperation()

	opCtx, cancel := context.WithTimeout(browserCtx, 10*time.Second)
	defer cancel()

	var html string
	err = chromedp.Run(opCtx,
		chromedp.OuterHTML(selector, &html, chromedp.AtLeast(0)),
	)
	if err != nil {
		// Element not found
		return false, nil
	}

	return html != "", nil
}

// Submit submits a form.
func (a *Actions) Submit(selector string) error {
	browserCtx, err := a.pool.GetContext()
	if err != nil {
		return err
	}

	a.pool.BeginOperation()
	defer a.pool.EndOperation()

	opCtx, cancel := context.WithTimeout(browserCtx, 30*time.Second)
	defer cancel()

	return chromedp.Run(opCtx,
		chromedp.Submit(selector),
		chromedp.WaitReady("body"),
	)
}

// SelectOption selects an option from a select element.
func (a *Actions) SelectOption(selector string, value string) error {
	browserCtx, err := a.pool.GetContext()
	if err != nil {
		return err
	}

	a.pool.BeginOperation()
	defer a.pool.EndOperation()

	opCtx, cancel := context.WithTimeout(browserCtx, 10*time.Second)
	defer cancel()

	return chromedp.Run(opCtx,
		chromedp.SetValue(selector, value),
	)
}

// SetCheckbox sets the checked state of a checkbox.
func (a *Actions) SetCheckbox(selector string, checked bool) error {
	browserCtx, err := a.pool.GetContext()
	if err != nil {
		return err
	}

	a.pool.BeginOperation()
	defer a.pool.EndOperation()

	opCtx, cancel := context.WithTimeout(browserCtx, 10*time.Second)
	defer cancel()

	return chromedp.Run(opCtx,
		chromedp.SetAttributeValue(selector, "checked", fmt.Sprintf("%t", checked)),
	)
}
