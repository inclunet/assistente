/**
 * Chat UI Constants
 * Centralized configuration for timing, dimensions, and other magic numbers
 */

// Timing constants (milliseconds)
export const TIMING = {
  DEBOUNCE_MS: 16, // ~60fps for input debouncing
  THROTTLE_MS: 50, // Backend stream throttling
  TOAST_DURATION: 2000, // Toast notification display time
  ANIMATION_DURATION: 200, // Standard animation/transition duration
  AUTO_FOCUS_INTERVAL: 100, // Polling interval for auto-focus
  AUTO_FOCUS_TIMEOUT: 2000, // Max time to wait for auto-focus
  ANNOUNCEMENT_CLEAR: 1000, // Time before clearing aria-live announcements
} as const;

// Dimension constants
export const DIMENSIONS = {
  TEXTAREA_MAX_HEIGHT: 200, // Max height for chat input textarea (px)
  MEDIA_PREVIEW_MAX_WIDTH: 150, // Max width for media preview thumbnails (px)
  TOOLBAR_BUTTON_MAX_WIDTH: 200, // Max width for toolbar button tooltips (px)
  MESSAGE_MAX_HEIGHT: 600, // Max height for message content before scroll (px)
} as const;

// Size limits
export const LIMITS = {
  MESSAGE_CONTENT_SIZE: 500 * 1024, // 500KB max message size
  MEDIA_SIZE: 10 * 1024 * 1024, // 10MB max media file size
  MESSAGE_PREVIEW_LENGTH: 150, // Character limit for message previews
  LONG_MESSAGE_THRESHOLD: 2000, // Show warning for messages longer than this
} as const;

// Keyboard shortcuts
export const SHORTCUTS = {
  NEW_TAB: 'Ctrl+N',
  PREV_TAB: 'Ctrl+P',
  HISTORY: 'Ctrl+H',
  MODELS: 'Ctrl+M',
  PROFILES: 'Ctrl+I',
  SPEAK_MESSAGE: 'Space',
  MESSAGE_DETAILS: 'Enter',
  HELP: '?',
} as const;

// Aria-live politeness levels
export const ARIA_LIVE = {
  POLITE: 'polite',
  ASSERTIVE: 'assertive',
  OFF: 'off',
} as const;

// Loading states
export const LOADING_STATES = {
  IDLE: 'idle',
  SENDING: 'sending',
  THINKING: 'thinking',
  LOADING: 'loading',
  STREAMING: 'streaming',
} as const;

// Message states
export const MESSAGE_STATES = {
  PENDING: 'pending',
  SENDING: 'sending',
  SENT: 'sent',
  FAILED: 'failed',
  STREAMING: 'streaming',
} as const;
