import { lazy, Suspense, useCallback, useEffect, useRef, type ComponentType } from 'react';
import { useTranslation } from 'react-i18next';
import {
  registerGoTemplateLanguage,
  GOTEMPLATE_LANGUAGE_ID,
  GOTEMPLATE_FUNCTIONS,
} from '../../../lib/gotemplate-language';
import './TemplateEditor.css';

const MonacoEditor = lazy(async () => {
  const mod = await import('@monaco-editor/react');
  return { default: mod.default as ComponentType<Record<string, unknown>> };
});

type MonacoApi = {
  languages: {
    CompletionItemKind: Record<string, number>;
    registerCompletionItemProvider: (
      languageId: string,
      provider: Record<string, unknown>,
    ) => { dispose: () => void };
  };
  Range: new (sl: number, sc: number, el: number, ec: number) => unknown;
  editor: { defineTheme: (name: string, data: Record<string, unknown>) => void };
};

type MonacoEditorApi = {
  updateOptions?: (opts: Record<string, unknown>) => void;
  onDidFocusEditorWidget?: (cb: () => void) => { dispose: () => void };
  onDidBlurEditorWidget?: (cb: () => void) => { dispose: () => void };
  getDomNode?: () => HTMLElement | null;
};

type MonacoModel = {
  getValueInRange: (range: {
    startLineNumber: number;
    startColumn: number;
    endLineNumber: number;
    endColumn: number;
  }) => string;
  getWordUntilPosition: (pos: { lineNumber: number; column: number }) => {
    startColumn: number;
    endColumn: number;
    word: string;
  };
};

export interface TemplateEditorContext {
  event?: Record<string, unknown>;
  output?: Record<string, unknown>;
}

export interface TemplateEditorProps {
  value: string;
  onChange: (value: string) => void;
  context?: TemplateEditorContext;
  height?: string;
  singleLine?: boolean;
  readOnly?: boolean;
  placeholder?: string;
  ariaLabel?: string;
}

function tryParseJSON(value: unknown): unknown {
  if (typeof value !== 'string') return value;
  const trimmed = value.trim();
  if ((trimmed.startsWith('{') && trimmed.endsWith('}')) ||
      (trimmed.startsWith('[') && trimmed.endsWith(']'))) {
    try {
      return JSON.parse(trimmed);
    } catch {
      return value;
    }
  }
  return value;
}

function flattenKeys(obj: Record<string, unknown>, prefix = '', depth = 0): string[] {
  if (depth > 6) return [];
  const keys: string[] = [];
  for (const [k, v] of Object.entries(obj)) {
    const path = prefix ? `${prefix}.${k}` : k;
    keys.push(path);
    const resolved = tryParseJSON(v);
    if (resolved && typeof resolved === 'object' && !Array.isArray(resolved)) {
      keys.push(...flattenKeys(resolved as Record<string, unknown>, path, depth + 1));
    }
    if (Array.isArray(resolved) && resolved.length > 0 && typeof resolved[0] === 'object' && resolved[0] !== null) {
      keys.push(...flattenKeys(resolved[0] as Record<string, unknown>, `${path}.0`, depth + 1));
    }
  }
  return keys;
}

function getTypeLabel(value: unknown): string {
  const resolved = tryParseJSON(value);
  if (resolved === null || resolved === undefined) return 'null';
  if (Array.isArray(resolved)) return 'array';
  if (typeof resolved === 'object') return 'object';
  return typeof resolved;
}

// --- Global singleton completion provider ---
// All TemplateEditor instances share a single provider.
// The active editor's context is resolved via activeContextRef.
const activeContextRef: { current: TemplateEditorContext | undefined } = { current: undefined };
let providerMonaco: MonacoApi | null = null;

function ensureCompletionProvider(monaco: MonacoApi) {
  if (providerMonaco === monaco) return;
  providerMonaco = monaco;

  monaco.languages.registerCompletionItemProvider(GOTEMPLATE_LANGUAGE_ID, {
    triggerCharacters: ['.', '|', ' '],
    provideCompletionItems: (model: MonacoModel, position: { lineNumber: number; column: number }) => {
      const textBefore = model.getValueInRange({
        startLineNumber: position.lineNumber,
        startColumn: Math.max(1, position.column - 200),
        endLineNumber: position.lineNumber,
        endColumn: position.column,
      });

      const word = model.getWordUntilPosition(position);
      const range = new monaco.Range(
        position.lineNumber,
        word.startColumn,
        position.lineNumber,
        word.endColumn,
      );

      const ctx = activeContextRef.current;

      const pipeMatch = textBefore.match(/\{\{[^}]*\|\s*$/);
      if (pipeMatch) {
        return {
          suggestions: GOTEMPLATE_FUNCTIONS.map((fn) => ({
            label: fn.name,
            kind: monaco.languages.CompletionItemKind.Function,
            documentation: fn.doc,
            detail: fn.detail,
            insertText: fn.name,
            range,
          })),
        };
      }

      const lastOpen = textBefore.lastIndexOf('{{');
      const lastClose = textBefore.lastIndexOf('}}');
      if (lastOpen < 0 || lastOpen <= lastClose) return { suggestions: [] };

      const exprText = textBefore.substring(lastOpen);

      const resolveFromCtx = (pathStr: string): Record<string, unknown> | undefined => {
        const parts = pathStr.split('.');
        const rootKey = parts[0];
        const restPath = parts.slice(1);

        let source: Record<string, unknown> | undefined;
        if (rootKey === 'event' && ctx?.event) source = ctx.event;
        else if (rootKey === 'output' && ctx?.output) source = ctx.output;
        if (!source) return undefined;

        let current: unknown = source;
        for (const part of restPath) {
          current = tryParseJSON(current);
          if (Array.isArray(current) && current.length > 0 && typeof current[0] === 'object' && current[0] !== null) {
            current = (current[0] as Record<string, unknown>)[part];
          } else if (current && typeof current === 'object' && !Array.isArray(current)) {
            current = (current as Record<string, unknown>)[part];
          } else {
            return undefined;
          }
        }

        current = tryParseJSON(current);

        if (Array.isArray(current) && current.length > 0 && typeof current[0] === 'object' && current[0] !== null) {
          current = current[0];
        }

        if (current && typeof current === 'object' && !Array.isArray(current)) {
          return current as Record<string, unknown>;
        }
        return undefined;
      };

      const nestedMatch = exprText.match(/\.(\w+(?:\.\w+)*)\.(\w*)\s*$/);
      if (nestedMatch) {
        const resolved = resolveFromCtx(nestedMatch[1]);
        if (resolved && Object.keys(resolved).length > 0) {
          return {
            suggestions: Object.keys(resolved).map((key) => ({
              label: key,
              kind: monaco.languages.CompletionItemKind.Field,
              detail: getTypeLabel(resolved[key]),
              insertText: key,
              range,
            })),
          };
        }
        return { suggestions: [] };
      }

      // Detect if there's already a `.` before the word — Go templates require `.field` syntax
      const charBeforeWord = textBefore.charAt(textBefore.length - word.word.length - 1);
      const dotPrefix = charBeforeWord === '.' ? '' : '.';

      // Root: always show event, output, now
      const suggestions: Array<Record<string, unknown>> = [
        { label: '.event', kind: monaco.languages.CompletionItemKind.Module, detail: 'Payload do evento trigger', insertText: `${dotPrefix}event`, range },
        { label: '.output', kind: monaco.languages.CompletionItemKind.Module, detail: 'Output da tool', insertText: `${dotPrefix}output`, range },
        { label: '.now', kind: monaco.languages.CompletionItemKind.Variable, detail: 'time.Time atual', insertText: `${dotPrefix}now`, range },
      ];

      if (ctx?.event && Object.keys(ctx.event).length > 0) {
        for (const key of flattenKeys(ctx.event)) {
          suggestions.push({
            label: `.event.${key}`,
            kind: monaco.languages.CompletionItemKind.Field,
            detail: 'event field',
            insertText: `${dotPrefix}event.${key}`,
            range,
          });
        }
      }
      if (ctx?.output && Object.keys(ctx.output).length > 0) {
        for (const key of flattenKeys(ctx.output)) {
          suggestions.push({
            label: `.output.${key}`,
            kind: monaco.languages.CompletionItemKind.Field,
            detail: 'output field',
            insertText: `${dotPrefix}output.${key}`,
            range,
          });
        }
      }

      return { suggestions };
    },
  });
}

export function TemplateEditor({
  value,
  onChange,
  context,
  height,
  singleLine = false,
  readOnly = false,
  placeholder = '',
  ariaLabel,
}: TemplateEditorProps) {
  const { t } = useTranslation();
  const contextRef = useRef(context);
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    contextRef.current = context;
  }, [context]);

  const resolvedHeight = height ?? (singleLine ? '38px' : '200px');
  const label = ariaLabel ?? t('jobs.builder.templateEditorLabel');

  const handleMount = useCallback(
    (editor: MonacoEditorApi, monacoRaw: unknown) => {
      const monaco = monacoRaw as MonacoApi;
      registerGoTemplateLanguage(monaco as never);
      ensureCompletionProvider(monaco);

      if (singleLine) {
        editor.updateOptions?.({
          lineNumbers: 'off',
          glyphMargin: false,
          folding: false,
          lineDecorationsWidth: 0,
          lineNumbersMinChars: 0,
          overviewRulerLanes: 0,
          renderLineHighlight: 'none',
          scrollbar: { vertical: 'hidden', horizontal: 'auto', handleMouseWheel: false },
          wordWrap: 'off',
          padding: { top: 8, bottom: 8 },
          tabFocusMode: true,
        });
      }

      editor.onDidFocusEditorWidget?.(() => {
        activeContextRef.current = contextRef.current;
        if (singleLine) {
          containerRef.current?.classList.add('template-editor--focused');
        }
      });

      if (singleLine) {
        editor.onDidBlurEditorWidget?.(() => {
          containerRef.current?.classList.remove('template-editor--focused');
        });
      }
    },
    [singleLine],
  );

  const handleChange = useCallback(
    (newValue: string | undefined) => {
      if (newValue === undefined) return;
      if (singleLine) {
        onChange(newValue.replace(/\n/g, ''));
      } else {
        onChange(newValue);
      }
    },
    [onChange, singleLine],
  );

  return (
    <div
      ref={containerRef}
      className={`template-editor ${singleLine ? 'template-editor--single-line' : ''}`}
      role="region"
      aria-label={label}
    >
      <Suspense fallback={<div className="template-editor__loading" aria-busy="true" />}>
        <MonacoEditor
          height={resolvedHeight}
          language={GOTEMPLATE_LANGUAGE_ID}
          value={value}
          onChange={handleChange}
          onMount={handleMount}
          theme="vs-dark"
          options={{
            minimap: { enabled: false },
            scrollBeyondLastLine: false,
            fontSize: singleLine ? 15 : 14,
            readOnly,
            wordWrap: singleLine ? 'off' : 'on',
            automaticLayout: true,
            autoClosingBrackets: 'always',
            quickSuggestions: { other: true, comments: false, strings: true },
            suggestOnTriggerCharacters: true,
            wordBasedSuggestions: 'off',
            accessibilitySupport: 'on',
            accessibilityPageSize: 10,
            ariaLabel: label,
            lineNumbers: singleLine ? 'off' : 'on',
            renderLineHighlight: singleLine ? 'none' : 'line',
            scrollbar: singleLine
              ? { vertical: 'hidden', horizontal: 'auto', handleMouseWheel: false }
              : undefined,
          }}
        />
      </Suspense>
      {!value && placeholder && (
        <div className="template-editor__placeholder">{placeholder}</div>
      )}
    </div>
  );
}
