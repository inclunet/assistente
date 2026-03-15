const loadedLanguages = new Set<string>();

const normalizeLanguage = (language: string) => {
  const value = String(language || '').trim().toLowerCase();
  if (!value) return 'plaintext';
  if (value === 'js') return 'javascript';
  if (value === 'ts') return 'typescript';
  if (value === 'yml') return 'yaml';
  if (value === 'bash' || value === 'sh') return 'shell';
  return value;
};

const languageLoaders: Record<string, () => Promise<unknown>> = {
  markdown: () => import('monaco-editor/esm/vs/basic-languages/markdown/markdown.contribution.js'),
  json: () => import('monaco-editor/esm/vs/language/json/monaco.contribution.js'),
  javascript: () => import('monaco-editor/esm/vs/language/typescript/monaco.contribution.js'),
  typescript: () => import('monaco-editor/esm/vs/language/typescript/monaco.contribution.js'),
  html: () => import('monaco-editor/esm/vs/language/html/monaco.contribution.js'),
  css: () => import('monaco-editor/esm/vs/language/css/monaco.contribution.js'),
  shell: () => import('monaco-editor/esm/vs/basic-languages/shell/shell.contribution.js'),
  yaml: () => import('monaco-editor/esm/vs/basic-languages/yaml/yaml.contribution.js'),
  python: () => import('monaco-editor/esm/vs/basic-languages/python/python.contribution.js'),
  go: () => import('monaco-editor/esm/vs/basic-languages/go/go.contribution.js'),
  sql: () => import('monaco-editor/esm/vs/basic-languages/sql/sql.contribution.js'),
};

export async function loadMonacoLanguage(language: string) {
  const normalized = normalizeLanguage(language);
  if (normalized === 'plaintext' || normalized === 'text') return;
  if (loadedLanguages.has(normalized)) return;

  const loader = languageLoaders[normalized];
  if (!loader) return;

  loadedLanguages.add(normalized);
  try {
    await loader();
  } catch {
    loadedLanguages.delete(normalized);
  }
}
