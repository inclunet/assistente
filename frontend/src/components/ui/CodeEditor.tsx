import Editor from '@monaco-editor/react';
import { buildMarkdownLinkFromSelection, normalizePastedLinkHref } from '../../lib/linkPaste';
import './CodeEditor.css';

export interface CodeEditorProps {
  value: string;
  onChange: (value: string) => void;
  height?: string;
  readOnly?: boolean;
  language?: string;
  placeholder?: string;
  ariaLabel?: string;
  theme?: string;
  onMount?: (editor: any, monaco: any) => void;
  pasteUrlAsMarkdownLink?: boolean;
}

export function CodeEditor({
  value,
  onChange,
  height = '300px',
  readOnly = false,
  language = 'plaintext',
  placeholder = '',
  ariaLabel = 'Editor de código',
  theme = 'vs-dark',
  onMount,
  pasteUrlAsMarkdownLink = false,
}: CodeEditorProps) {
  const handleEditorChange = (newValue: string | undefined) => {
    if (newValue !== undefined) onChange(newValue);
  };

  const handleMount = (editor: any, monaco: any) => {
    onMount?.(editor, monaco);

    if (!pasteUrlAsMarkdownLink) return;

    const domNode: HTMLElement | null = editor?.getDomNode?.() ?? null;
    const model = editor?.getModel?.();
    if (!domNode || !model) return;

    const onPaste = (e: ClipboardEvent) => {
      if (readOnly) return;
      const href = normalizePastedLinkHref(e.clipboardData?.getData('text/plain') ?? '');
      if (!href) return;

      const selection = editor?.getSelection?.();
      if (!selection || selection.isEmpty?.()) return;

      const selectedText = String(model.getValueInRange(selection) ?? '');
      if (!selectedText) return;

      e.preventDefault();
      const replacement = buildMarkdownLinkFromSelection({ selectedText, href });
      editor.executeEdits('paste-url-as-markdown-link', [
        {
          range: selection,
          text: replacement,
          forceMoveMarkers: true,
        },
      ]);
    };

    domNode.addEventListener('paste', onPaste, true);
    editor?.onDidDispose?.(() => {
      domNode.removeEventListener('paste', onPaste, true);
    });
  };

  return (
    <div className="code-editor" role="region" aria-label={ariaLabel}>
      <Editor
        height={height}
        language={language}
        value={value}
        onChange={handleEditorChange}
        onMount={handleMount}
        theme={theme}
        options={{
          minimap: { enabled: false },
          scrollBeyondLastLine: false,
          fontSize: 14,
          readOnly,
          wordWrap: 'on',
          formatOnPaste: true,
          formatOnType: true,
          automaticLayout: true,
          accessibilitySupport: 'on',
          accessibilityPageSize: 10,
          ariaLabel,
        }}
      />
      {!value && placeholder && <div className="code-editor__placeholder">{placeholder}</div>}
    </div>
  );
}
