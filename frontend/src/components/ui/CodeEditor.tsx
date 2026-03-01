import Editor from '@monaco-editor/react';
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
}: CodeEditorProps) {
  const handleEditorChange = (newValue: string | undefined) => {
    if (newValue !== undefined) onChange(newValue);
  };

  return (
    <div className="code-editor" role="region" aria-label={ariaLabel}>
      <Editor
        height={height}
        language={language}
        value={value}
        onChange={handleEditorChange}
        onMount={onMount}
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
