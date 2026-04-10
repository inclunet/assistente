// Type declarations for Wails v2→v3 compatibility shim
// Only contains symbols used in this codebase.

/**
 * Subscribe to a named event emitted from Go.
 * @returns A function that, when called, unsubscribes the listener.
 */
export declare function EventsOn(eventName: string, callback: (data: any) => void): () => void;

/**
 * Open a URL in the system default browser.
 */
export declare function BrowserOpenURL(url: string): void;

/**
 * Set the title of the current window.
 */
export declare function WindowSetTitle(title: string): void;
