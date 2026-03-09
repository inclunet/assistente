import '@testing-library/jest-dom';
import { vi } from 'vitest';

if (typeof document !== 'undefined' && !document.queryCommandSupported) {
	document.queryCommandSupported = () => false;
}

vi.mock('monaco-editor', () => ({}));
vi.mock('monaco-editor/esm/vs/editor/editor.api', () => ({}));
vi.mock('monaco-editor/esm/vs/editor/editor.api.js', () => ({}));
vi.mock('monaco-editor/esm/vs/editor/editor.main', () => ({}));
vi.mock('monaco-editor/esm/vs/editor/editor.main.js', () => ({}));
vi.mock('monaco-editor/esm/vs/editor/contrib/clipboard/browser/clipboard.js', () => ({}));
vi.mock('monaco-editor/esm/vs/language/css/monaco.contribution.js', () => ({}));
