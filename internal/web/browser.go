// Package web provides web navigation and content extraction capabilities
// using chromedp for browser automation and goquery for HTML parsing.
package web

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chromedp/chromedp"
)

// BrowserPool manages a pool of browser instances with automatic lifecycle management.
// It starts browsers on-demand and closes them after a period of inactivity.
type BrowserPool struct {
	allocCtx      context.Context
	allocCancel   context.CancelFunc
	browserCtx    context.Context
	browserCancel context.CancelFunc

	mu          sync.Mutex
	lastUsed    time.Time
	idleTimer   *time.Timer
	idleTimeout time.Duration
	activeOps   int32 // contador de operações ativas (atomic)

	visible bool // headless vs visible mode
	running bool
}

// NewBrowserPool creates a new browser pool with the specified idle timeout.
// The browser will be closed after idleTimeout of inactivity.
func NewBrowserPool(idleTimeout time.Duration) *BrowserPool {
	if idleTimeout <= 0 {
		idleTimeout = 5 * time.Minute
	}

	return &BrowserPool{
		idleTimeout: idleTimeout,
		visible:     false,
	}
}

// GetContext returns a browser context, starting the browser if necessary.
// The context can be used with chromedp.Run() for browser operations.
func (p *BrowserPool) GetContext() (context.Context, error) {
	fmt.Printf("🌐 [BrowserPool] GetContext chamado\n")

	p.mu.Lock()
	defer p.mu.Unlock()

	// Reset idle timer
	p.resetIdleTimerLocked()

	// Return existing context if available
	if p.running && p.browserCtx != nil {
		fmt.Printf("🌐 [BrowserPool] Retornando contexto existente (running=%v)\n", p.running)
		return p.browserCtx, nil
	}

	fmt.Printf("🌐 [BrowserPool] Browser não está rodando, iniciando...\n")

	// Start new browser
	if err := p.startBrowserLocked(); err != nil {
		fmt.Printf("❌ [BrowserPool] Erro ao iniciar browser: %v\n", err)
		return nil, fmt.Errorf("failed to start browser: %w", err)
	}

	fmt.Printf("✅ [BrowserPool] Browser iniciado com sucesso\n")
	return p.browserCtx, nil
}

// SetVisible sets the browser visibility mode.
// If the mode changes and the browser is running, it will be restarted.
func (p *BrowserPool) SetVisible(visible bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.visible == visible {
		return nil
	}

	p.visible = visible

	// Need to restart browser if running
	if p.running {
		p.stopBrowserLocked()
	}

	return nil
}

// IsVisible returns whether the browser is in visible mode.
func (p *BrowserPool) IsVisible() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.visible
}

// IsRunning returns whether the browser is currently running.
func (p *BrowserPool) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running
}

// Close closes the browser and releases resources.
func (p *BrowserPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopBrowserLocked()
}

// startBrowserLocked starts a new browser instance with retry logic.
// Must be called with mutex locked.
func (p *BrowserPool) startBrowserLocked() error {
	var lastErr error
	maxRetries := 3

	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := p.tryStartBrowser()
		if err == nil {
			fmt.Printf("🌐 [BrowserPool] Browser iniciado com sucesso (tentativa %d)\n", attempt)
			return nil
		}

		lastErr = err
		fmt.Printf("⚠️ [BrowserPool] Falha ao iniciar browser (tentativa %d/%d): %v\n", attempt, maxRetries, err)

		// Clean up failed attempt
		p.stopBrowserLocked()

		// Wait before retry (except on last attempt)
		if attempt < maxRetries {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}

	return fmt.Errorf("failed to start browser after %d attempts: %w", maxRetries, lastErr)
}

// tryStartBrowser attempts to start the browser once.
func (p *BrowserPool) tryStartBrowser() error {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		// Disable downloads for security
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("disable-extensions", true),

		// Privacy settings
		chromedp.Flag("disable-sync", true),

		// Disable notifications/popups
		chromedp.Flag("disable-notifications", true),

		// Disable GPU for headless mode stability
		chromedp.Flag("disable-gpu", true),

		// Useful for some sites
		chromedp.Flag("disable-web-security", false),
		chromedp.Flag("disable-features", "IsolateOrigins,site-per-process"),

		// Window size
		chromedp.WindowSize(1920, 1080),

		// Longer timeouts for slow connections
		chromedp.Flag("disable-background-timer-throttling", true),
		chromedp.Flag("disable-backgrounding-occluded-windows", true),
		chromedp.Flag("disable-renderer-backgrounding", true),

		// Disable sandbox for compatibility (may help in some environments)
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-setuid-sandbox", true),
	)

	// Set headless mode based on visibility setting
	if p.visible {
		opts = append(opts,
			chromedp.Flag("headless", false),
		)
	} else {
		opts = append(opts,
			chromedp.Flag("headless", true),
		)
	}

	// Create allocator context
	p.allocCtx, p.allocCancel = chromedp.NewExecAllocator(context.Background(), opts...)

	// Create browser context
	var browserCtx context.Context
	browserCtx, p.browserCancel = chromedp.NewContext(p.allocCtx,
		chromedp.WithLogf(func(format string, args ...interface{}) {
			// Suppress chromedp logs in production
		}),
	)
	p.browserCtx = browserCtx

	// Run a simple command to actually start the browser
	// IMPORTANT: Don't use a timeout context here because canceling it
	// can interfere with chromedp's internal state. The browser startup
	// should complete quickly anyway.
	if err := chromedp.Run(p.browserCtx); err != nil {
		return fmt.Errorf("failed to initialize browser: %w", err)
	}

	p.running = true
	p.lastUsed = time.Now()

	return nil
}

// stopBrowserLocked stops the browser and cleans up resources.
// Must be called with mutex locked.
func (p *BrowserPool) stopBrowserLocked() {
	if p.idleTimer != nil {
		p.idleTimer.Stop()
		p.idleTimer = nil
	}

	if p.browserCancel != nil {
		p.browserCancel()
		p.browserCancel = nil
		p.browserCtx = nil
	}

	if p.allocCancel != nil {
		p.allocCancel()
		p.allocCancel = nil
		p.allocCtx = nil
	}

	p.running = false
}

// resetIdleTimerLocked resets the idle timer.
// Must be called with mutex locked.
func (p *BrowserPool) resetIdleTimerLocked() {
	if p.idleTimer != nil {
		p.idleTimer.Stop()
	}

	p.lastUsed = time.Now()
	p.idleTimer = time.AfterFunc(p.idleTimeout, func() {
		p.mu.Lock()
		defer p.mu.Unlock()

		// Don't close if there are active operations
		if atomic.LoadInt32(&p.activeOps) > 0 {
			fmt.Printf("🌐 [BrowserPool] Timer disparou mas há %d operações ativas, adiando fechamento\n", p.activeOps)
			// Reset timer to check again later
			p.resetIdleTimerLocked()
			return
		}

		// Double check that we're still idle
		if time.Since(p.lastUsed) >= p.idleTimeout {
			fmt.Printf("🌐 [BrowserPool] Fechando browser por inatividade\n")
			p.stopBrowserLocked()
		}
	})
}

// BeginOperation marks the start of a browser operation.
// This prevents the idle timer from closing the browser during the operation.
func (p *BrowserPool) BeginOperation() {
	atomic.AddInt32(&p.activeOps, 1)
}

// EndOperation marks the end of a browser operation.
func (p *BrowserPool) EndOperation() {
	atomic.AddInt32(&p.activeOps, -1)
	// Update last used time
	p.mu.Lock()
	p.lastUsed = time.Now()
	p.mu.Unlock()
}

// Navigate navigates to a URL and waits for the page to load.
func (p *BrowserPool) Navigate(url string, timeout time.Duration) error {
	browserCtx, err := p.GetContext()
	if err != nil {
		return err
	}

	p.BeginOperation()
	defer p.EndOperation()

	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	opCtx, cancel := context.WithTimeout(browserCtx, timeout)
	defer cancel()

	return chromedp.Run(opCtx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body"),
	)
}

// GetHTML returns the full HTML of the current page.
func (p *BrowserPool) GetHTML() (string, error) {
	browserCtx, err := p.GetContext()
	if err != nil {
		return "", err
	}

	p.BeginOperation()
	defer p.EndOperation()

	opCtx, cancel := context.WithTimeout(browserCtx, 30*time.Second)
	defer cancel()

	var html string
	if err := chromedp.Run(opCtx, chromedp.OuterHTML("html", &html)); err != nil {
		return "", fmt.Errorf("failed to get HTML: %w", err)
	}

	return html, nil
}

// GetCurrentURL returns the current page URL.
func (p *BrowserPool) GetCurrentURL() (string, error) {
	browserCtx, err := p.GetContext()
	if err != nil {
		return "", err
	}

	p.BeginOperation()
	defer p.EndOperation()

	opCtx, cancel := context.WithTimeout(browserCtx, 10*time.Second)
	defer cancel()

	var url string
	if err := chromedp.Run(opCtx, chromedp.Location(&url)); err != nil {
		return "", fmt.Errorf("failed to get URL: %w", err)
	}

	return url, nil
}

// GetTitle returns the current page title.
func (p *BrowserPool) GetTitle() (string, error) {
	browserCtx, err := p.GetContext()
	if err != nil {
		return "", err
	}

	p.BeginOperation()
	defer p.EndOperation()

	opCtx, cancel := context.WithTimeout(browserCtx, 10*time.Second)
	defer cancel()

	var title string
	if err := chromedp.Run(opCtx, chromedp.Title(&title)); err != nil {
		return "", fmt.Errorf("failed to get title: %w", err)
	}

	return title, nil
}

// WaitFor waits for an element to be visible.
func (p *BrowserPool) WaitFor(selector string, timeout time.Duration) error {
	browserCtx, err := p.GetContext()
	if err != nil {
		return err
	}

	p.BeginOperation()
	defer p.EndOperation()

	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	opCtx, cancel := context.WithTimeout(browserCtx, timeout)
	defer cancel()

	return chromedp.Run(opCtx, chromedp.WaitVisible(selector))
}

// Sleep pauses for the specified duration. Useful for waiting for dynamic content.
func (p *BrowserPool) Sleep(duration time.Duration) error {
	browserCtx, err := p.GetContext()
	if err != nil {
		return err
	}

	p.BeginOperation()
	defer p.EndOperation()

	// Use timeout slightly longer than sleep duration
	opCtx, cancel := context.WithTimeout(browserCtx, duration+5*time.Second)
	defer cancel()

	return chromedp.Run(opCtx, chromedp.Sleep(duration))
}
