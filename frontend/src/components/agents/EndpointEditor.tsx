import { useState, useEffect } from 'react';
import { JsonEditor } from '../ui/JsonEditor';
import { Input } from '../ui/Input';
import { Textarea } from '../ui/Textarea';
import { Select } from '../ui/Select';
import { Button } from '../ui/Button';
import { EndpointTester } from './EndpointTester';
import './EndpointEditor.css';

interface EndpointEditorProps {
  endpoint?: any;
  agentId?: number;
  onSave: (endpointData: any) => Promise<void>;
  onCancel: () => void;
  onTest?: (endpointData: any) => void;
}

const HTTP_METHODS = [
  { value: 'GET', label: 'GET', color: '#61affe' },
  { value: 'POST', label: 'POST', color: '#49cc90' },
  { value: 'PUT', label: 'PUT', color: '#fca130' },
  { value: 'PATCH', label: 'PATCH', color: '#50e3c2' },
  { value: 'DELETE', label: 'DELETE', color: '#f93e3e' },
];

const DEFAULT_SCHEMA = {
  type: 'object',
  properties: {},
  required: []
};

export function EndpointEditor({ endpoint, agentId, onSave, onCancel, onTest }: EndpointEditorProps) {
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  // Form state
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [method, setMethod] = useState('GET');
  const [pathTemplate, setPathTemplate] = useState('');
  const [queryTemplate, setQueryTemplate] = useState('');
  const [headersJSON, setHeadersJSON] = useState('{}');
  const [bodyTemplate, setBodyTemplate] = useState('');
  const [parametersJSON, setParametersJSON] = useState(JSON.stringify(DEFAULT_SCHEMA, null, 2));
  const [responseTemplate, setResponseTemplate] = useState('');

  // Variáveis disponíveis extraídas do JSON Schema
  const [availableVariables, setAvailableVariables] = useState<string[]>([]);

  useEffect(() => {
    if (endpoint) {
      setName(endpoint.name || '');
      setDescription(endpoint.description || '');
      setMethod(endpoint.method || 'GET');
      setPathTemplate(endpoint.path_template || '');
      setQueryTemplate(endpoint.query_template || '');
      setHeadersJSON(endpoint.headers_json || '{}');
      setBodyTemplate(endpoint.body_template || '');
      setResponseTemplate(endpoint.response_template || '');

      try {
        const params = endpoint.parameters ? JSON.parse(endpoint.parameters) : DEFAULT_SCHEMA;
        setParametersJSON(JSON.stringify(params, null, 2));
      } catch {
        setParametersJSON(JSON.stringify(DEFAULT_SCHEMA, null, 2));
      }
    }
  }, [endpoint]);

  // Extrair variáveis disponíveis do JSON Schema
  useEffect(() => {
    try {
      const params = JSON.parse(parametersJSON);
      if (params.properties && typeof params.properties === 'object') {
        const variables = Object.keys(params.properties);
        setAvailableVariables(variables);
      } else {
        setAvailableVariables([]);
      }
    } catch {
      setAvailableVariables([]);
    }
  }, [parametersJSON]);

  const handleSave = async () => {
    if (!name.trim()) {
      setError('Nome é obrigatório');
      return;
    }
    if (!pathTemplate.trim()) {
      setError('Path template é obrigatório');
      return;
    }

    // Valida JSON do parameters
    try {
      JSON.parse(parametersJSON);
    } catch {
      setError('Schema de parâmetros inválido. Deve ser um JSON válido.');
      return;
    }

    // Valida JSON dos headers
    try {
      JSON.parse(headersJSON);
    } catch {
      setError('Headers JSON inválido. Deve ser um JSON válido.');
      return;
    }

    setSaving(true);
    setError('');

    const endpointData = {
      id: endpoint?.id,
      name: name.trim(),
      description: description.trim(),
      method,
      path_template: pathTemplate,
      query_template: queryTemplate,
      headers_json: headersJSON,
      body_template: bodyTemplate,
      parameters: parametersJSON,
      response_template: responseTemplate,
    };

    try {
      await onSave(endpointData);
    } catch (err: any) {
      setError('Erro ao salvar: ' + (err.message || err));
    } finally {
      setSaving(false);
    }
  };

  const handleTest = () => {
    if (onTest) {
      onTest({
        name,
        method,
        path_template: pathTemplate,
        query_template: queryTemplate,
        body_template: bodyTemplate,
        headers_json: headersJSON,
        parameters: parametersJSON,
      });
    }
  };

  return (
    <div className="endpoint-editor">
      <div className="editor-header">
        <h3>{endpoint?.id ? 'Editar Endpoint' : 'Novo Endpoint'}</h3>
      </div>

      {error && <div className="error-message" role="alert">{error}</div>}

      {/* Identificação */}
      <div className="form-section">
        <h4>Identificação</h4>

        <div className="form-row">
          <div className="form-group flex-1">
            <label htmlFor="endpoint-name">Nome da Função *</label>
            <Input
              id="endpoint-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="get_customer, create_order, etc."
              className="input-mono"
            />
            <small>Identificador único usado pelo LLM para chamar este endpoint</small>
          </div>

          <div className="form-group" style={{ width: '140px' }}>
            <label htmlFor="endpoint-method">Método *</label>
            <Select
              id="endpoint-method"
              value={method}
              onChange={(e) => setMethod(e.target.value)}
              options={HTTP_METHODS}
            />
          </div>
        </div>

        <div className="form-group">
          <label htmlFor="endpoint-description">Descrição</label>
          <Textarea
            id="endpoint-description"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={2}
            placeholder="Descreva o que este endpoint faz. Esta descrição ajuda o LLM a decidir quando usar."
          />
        </div>
      </div>

      {/* Request */}
      <div className="form-section">
        <h4>Request</h4>

        <div className="form-group">
          <label>Path Template *</label>
          <Input
            value={pathTemplate}
            onChange={(e) => setPathTemplate(e.target.value)}
            placeholder="/users/{{.user_id}}/orders"
            className="input-mono"
          />
          <small>
            Caminho da URL. Use {'{{'}.variavel{'}}'}para parâmetros dinâmicos.
            {availableVariables.length > 0 && (
              <strong> • Disponíveis: {availableVariables.join(', ')}</strong>
            )}
          </small>
        </div>

        <div className="form-group">
          <label>Query Template <span className="optional">(opcional)</span></label>
          <Input
            value={queryTemplate}
            onChange={(e) => setQueryTemplate(e.target.value)}
            placeholder="page={{.page | default 1}}&limit={{.limit | default 10}}"
            className="input-mono"
          />
          <small>
            Query string. Não inclua o "?". Use pipes: | default, | urlquery
            {availableVariables.length > 0 && (
              <strong> • Disponíveis: {availableVariables.join(', ')}</strong>
            )}
          </small>
        </div>

        {method !== 'GET' && method !== 'DELETE' && (
          <div className="form-group">
            <label>Body Template <span className="optional">(opcional)</span></label>
            <JsonEditor
              value={bodyTemplate}
              onChange={setBodyTemplate}
              height="150px"
              language="plaintext"
              placeholder='{\n  "name": "{{.name}}",\n  "email": "{{.email}}"\n}'
              templateVariables={availableVariables}
              modelId={`body-${endpoint?.id || 'new'}`}
            />
            <small>
              Corpo da requisição. Use {'{{'}.variavel{'}}'}para inserir dados.
              {availableVariables.length > 0 && (
                <strong> • Digite {'{{.'} para ver variáveis disponíveis</strong>
              )}
            </small>
          </div>
        )}

        <div className="form-group">
          <label>Headers Específicos <span className="optional">(opcional)</span></label>
          <JsonEditor
            value={headersJSON}
            onChange={setHeadersJSON}
            height="100px"
            language="json"
            placeholder='{\n  "X-Custom-Header": "valor"\n}'
          />
          <small>Headers adicionais para este endpoint. Serão mesclados com os headers padrão.</small>
        </div>
      </div>

      {/* Parâmetros (Schema) */}
      <div className="form-section">
        <h4>Parâmetros (Schema JSON)</h4>
        <p className="section-description">
          Define os parâmetros que o LLM pode passar para este endpoint usando JSON Schema.
          Estes parâmetros ficam disponíveis nos templates acima.
        </p>

        <div className="form-group">
          <JsonEditor
            value={parametersJSON}
            onChange={setParametersJSON}
            height="250px"
            language="json"
            jsonSchema={{
              type: 'object',
              properties: {
                type: { type: 'string', enum: ['object', 'string', 'number', 'boolean', 'array'] },
                properties: { type: 'object' },
                required: { type: 'array', items: { type: 'string' } },
                description: { type: 'string' },
                enum: { type: 'array' },
                default: {},
                items: { type: 'object' },
                additionalProperties: { type: 'boolean' },
              },
              required: ['type']
            }}
            modelId={`params-${endpoint?.id || 'new'}`}
            placeholder='{\n  "type": "object",\n  "properties": {\n    "user_id": {\n      "type": "string",\n      "description": "ID do usuário"\n    }\n  },\n  "required": ["user_id"]\n}'
          />
          <small>
            ✓ Validação de schema ativa! Define os parâmetros que ficam disponíveis como variáveis nos templates.
            <a href="https://json-schema.org/" target="_blank" rel="noopener noreferrer"> Saiba mais sobre JSON Schema</a>
          </small>
        </div>
      </div>

      {/* Response */}
      <div className="form-section">
        <h4>Response</h4>

        <div className="form-group">
          <label>Response Template <span className="optional">(opcional)</span></label>
          <Input
            value={responseTemplate}
            onChange={(e) => setResponseTemplate(e.target.value)}
            placeholder="Cliente {{.name}} encontrado com ID {{.id}}"
            className="input-mono"
          />
          <small>Template para formatar a resposta da API. As variáveis da resposta JSON ficam disponíveis.</small>
        </div>
      </div>

      {/* Endpoint Tester */}
      {endpoint?.id && agentId && (
        <EndpointTester
          agentId={agentId}
          endpointName={name}
          parametersSchema={parametersJSON}
        />
      )}

      {/* Actions */}
      <div className="form-actions">
        <Button variant="secondary" onClick={onCancel} disabled={saving}>
          Cancelar
        </Button>
        {onTest && (
          <Button variant="secondary" onClick={handleTest} disabled={saving}>
            🧪 Testar
          </Button>
        )}
        <Button variant="primary" onClick={handleSave} disabled={saving}>
          {saving ? 'Salvando...' : 'Salvar Endpoint'}
        </Button>
      </div>
    </div>
  );
}
