package web

import (
	"context"
	"time"

	"github.com/chromedp/chromedp"
)

// Screenshot provides methods for capturing screenshots.
type Screenshot struct {
	pool *BrowserPool
}

// NewScreenshot creates a new Screenshot instance.
func NewScreenshot(pool *BrowserPool) *Screenshot {
	return &Screenshot{pool: pool}
}

// CaptureViewport captures a screenshot of the current viewport.
func (s *Screenshot) CaptureViewport() ([]byte, error) {
	browserCtx, err := s.pool.GetContext()
	if err != nil {
		return nil, err
	}

	s.pool.BeginOperation()
	defer s.pool.EndOperation()

	opCtx, cancel := context.WithTimeout(browserCtx, 30*time.Second)
	defer cancel()

	var buf []byte
	err = chromedp.Run(opCtx, chromedp.CaptureScreenshot(&buf))
	if err != nil {
		return nil, err
	}

	return buf, nil
}

// CaptureFullPage captures a screenshot of the full page (scrolling).
func (s *Screenshot) CaptureFullPage() ([]byte, error) {
	browserCtx, err := s.pool.GetContext()
	if err != nil {
		return nil, err
	}

	s.pool.BeginOperation()
	defer s.pool.EndOperation()

	opCtx, cancel := context.WithTimeout(browserCtx, 60*time.Second)
	defer cancel()

	var buf []byte
	err = chromedp.Run(opCtx, chromedp.FullScreenshot(&buf, 90))
	if err != nil {
		return nil, err
	}

	return buf, nil
}

// CaptureElement captures a screenshot of a specific element.
func (s *Screenshot) CaptureElement(selector string) ([]byte, error) {
	browserCtx, err := s.pool.GetContext()
	if err != nil {
		return nil, err
	}

	s.pool.BeginOperation()
	defer s.pool.EndOperation()

	opCtx, cancel := context.WithTimeout(browserCtx, 30*time.Second)
	defer cancel()

	var buf []byte
	err = chromedp.Run(opCtx,
		chromedp.WaitVisible(selector),
		chromedp.Screenshot(selector, &buf, chromedp.NodeVisible),
	)
	if err != nil {
		return nil, err
	}

	return buf, nil
}

// CaptureWithTimeout captures a screenshot with a timeout.
func (s *Screenshot) CaptureWithTimeout(fullPage bool, timeout time.Duration) ([]byte, error) {
	ctx, err := s.pool.GetContext()
	if err != nil {
		return nil, err
	}

	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var buf []byte
	var action chromedp.Action

	if fullPage {
		action = chromedp.FullScreenshot(&buf, 90)
	} else {
		action = chromedp.CaptureScreenshot(&buf)
	}

	err = chromedp.Run(ctx, action)
	if err != nil {
		return nil, err
	}

	return buf, nil
}
