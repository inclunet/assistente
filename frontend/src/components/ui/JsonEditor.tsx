import { useRef, useEffect } from 'react';
import { CodeEditor } from './CodeEditor';
import './JsonEditor.css';

interface JsonEditorProps {
  value: string;
  onChange: (value: string) => void;
  height?: string;
  readOnly?: boolean;
  language?: 'json' | 'javascript' | 'typescript' | 'plaintext';
  placeholder?: string;
  // Para validação de JSON Schema
  jsonSchema?: object;
  // Para autocomplete de variáveis em templates
  templateVariables?: string[];
  // Identificador único para o modelo
  modelId?: string;
}

export function JsonEditor({ 
  value, 
  onChange, 
  height = '300px', 
  readOnly = false,
  language = 'json',
  placeholder = '',
  jsonSchema,
  templateVariables,
  modelId = 'default'
}: JsonEditorProps) {
  const editorRef = useRef<any>(null);
  const monacoRef = useRef<any>(null);

  useEffect(() => {
    if (!monacoRef.current) return;

    // Configurar validação de JSON Schema
    if (language === 'json' && jsonSchema) {
      monacoRef.current.languages.json.jsonDefaults.setDiagnosticsOptions({
        validate: true,
        schemas: [{
          uri: `http://myserver/${modelId}-schema.json`,
          fileMatch: ['*'],
          schema: jsonSchema
        }],
        enableSchemaRequest: false,
        allowComments: false,
      });
    }
  }, [language, jsonSchema, modelId]);

  const handleEditorDidMount = (editor: any, monaco: any) => {
    editorRef.current = editor;
    monacoRef.current = monaco;
    
    // Configurações do editor com acessibilidade
    editor.updateOptions({
      minimap: { enabled: false },
      wordWrap: 'on',
      formatOnPaste: true,
      formatOnType: true,
      scrollBeyondLastLine: false,
      fontSize: 14,
      lineNumbers: 'on',
      automaticLayout: true,
      // Opções de acessibilidade
      accessibilitySupport: 'on', // Ativa suporte a leitores de tela
      accessibilityPageSize: 10, // Número de linhas lidas por vez
      ariaLabel: 'Editor de código JSON', // Label para leitores de tela
    });

    // Registrar autocomplete para variáveis de template
    if (templateVariables && templateVariables.length > 0) {
      const disposable = monaco.languages.registerCompletionItemProvider(
        language === 'plaintext' ? 'plaintext' : 'json',
        {
          triggerCharacters: ['.'],
          provideCompletionItems: (model: any, position: any) => {
            const textUntilPosition = model.getValueInRange({
              startLineNumber: position.lineNumber,
              startColumn: Math.max(1, position.column - 20),
              endLineNumber: position.lineNumber,
              endColumn: position.column,
            });

            // Detectar se estamos dentro de um template {{.
            if (!textUntilPosition.includes('{{.')) {
              return { suggestions: [] };
            }

            const word = model.getWordUntilPosition(position);
            const range = {
              startLineNumber: position.lineNumber,
              endLineNumber: position.lineNumber,
              startColumn: word.startColumn,
              endColumn: word.endColumn,
            };

            const suggestions = templateVariables.map((variable) => ({
              label: variable,
              kind: monaco.languages.CompletionItemKind.Variable,
              documentation: `Variável disponível: ${variable}`,
              detail: 'template variable',
              insertText: variable,
              range: range,
            }));

            return { suggestions };
          },
        }
      );

      // Cleanup
      return () => {
        if (disposable) disposable.dispose();
      };
    }
  };

  return (
    <div className="json-editor" role="region" aria-label="Editor de JSON">
      <CodeEditor
        height={height}
        language={language}
        value={value}
        onChange={onChange}
        onMount={handleEditorDidMount}
        theme="vs-dark"
        readOnly={readOnly}
        placeholder={placeholder}
        ariaLabel={readOnly ? 'Editor de código somente leitura' : 'Editor de código'}
      />
    </div>
  );
}
