// Compatibility shim: Wails v2 API → Wails v3 (@wailsio/runtime)
// Only re-exports the symbols actually used in this codebase.
// This file is intentionally small — add wrappers here as needed.

import { Events, Browser, Window } from '@wailsio/runtime';

/**
 * EventsOn: subscribe to a named event.
 * v2: callback(data)
 * v3: callback(ev) where ev.data is the payload — wrapped here.
 * @returns unsubscribe function
 */
export function EventsOn(eventName, callback) {
    return Events.On(eventName, (ev) => callback(ev.data));
}

/**
 * BrowserOpenURL: open a URL in the system browser.
 */
export function BrowserOpenURL(url) {
    Browser.OpenURL(url);
}

/**
 * WindowSetTitle: set the window title.
 */
export function WindowSetTitle(title) {
    Window.SetTitle(title);
}
