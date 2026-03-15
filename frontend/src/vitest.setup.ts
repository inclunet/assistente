import '@testing-library/jest-dom';
import { vi } from 'vitest';
import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import { TTSProvider } from './services/tts/types';

if (!i18n.isInitialized) {
	void i18n.use(initReactI18next).init({
		lng: 'en',
		fallbackLng: 'en',
		resources: { en: { translation: {} } },
		interpolation: { escapeValue: false },
		initImmediate: false,
	});
}

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
vi.mock('monaco-editor/esm/vs/language/json/monaco.contribution.js', () => ({}));
vi.mock('monaco-editor/esm/vs/language/typescript/monaco.contribution.js', () => ({}));
vi.mock('monaco-editor/esm/vs/language/html/monaco.contribution.js', () => ({}));
vi.mock('monaco-editor/esm/vs/basic-languages/markdown/markdown.contribution.js', () => ({}));
vi.mock('monaco-editor/esm/vs/basic-languages/shell/shell.contribution.js', () => ({}));
vi.mock('monaco-editor/esm/vs/basic-languages/yaml/yaml.contribution.js', () => ({}));
vi.mock('monaco-editor/esm/vs/basic-languages/python/python.contribution.js', () => ({}));
vi.mock('monaco-editor/esm/vs/basic-languages/go/go.contribution.js', () => ({}));
vi.mock('monaco-editor/esm/vs/basic-languages/sql/sql.contribution.js', () => ({}));

vi.mock('./services/tts/factory', () => {
	const provider = {
		name: TTSProvider.WEBSPEECH,
		isAvailable: true,
		initialize: async () => {},
		getVoices: async () => [],
		setVoice: async () => {},
		setRate: async () => {},
		setVolume: async () => {},
		setPitch: () => {},
		speak: async () => {},
		stop: () => {},
		pause: () => {},
		resume: () => {},
		isSpeaking: () => false,
		dispose: () => {},
		addEventListener: () => {},
		removeEventListener: () => {},
	};

	return {
		ttsFactory: {
			initialize: async () => {},
			getProviderWithFallback: () => provider,
			getProvider: () => provider,
			getAvailableProviders: () => [TTSProvider.WEBSPEECH],
			isProviderAvailable: () => true,
			getProviderByVoiceName: async () => provider,
			dispose: () => {},
		},
	};
});
