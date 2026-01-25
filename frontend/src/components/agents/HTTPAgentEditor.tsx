import { useState, useEffect } from 'react';
import {
  CreateHTTPAgentFull,
  GetHTTPAgentFull,
  UpdateHTTPAgentFull,
  DeleteHTTPAgentFull,
  GetHTTPEndpointsByAgentID,
  CreateHTTPEndpoint,
  UpdateHTTPEndpoint,
  DeleteHTTPEndpoint,
  TestHTTPEndpoint,
} from '../../../wailsjs/go/main/App';
import { Input } from '../ui/Input';
import { Textarea } from '../ui/Textarea';
import { Select } from '../ui/Select';
import { Checkbox } from '../ui/Checkbox';
import { Button } from '../ui/Button';
import { Modal } from '../ui/Modal';
import { EndpointEditor } from './EndpointEditor';
import './HTTPAgentEditor.css';

interface HTTPAgentEditorProps {
  agentConfigId?: number | null;
  onClose?: () => void;
  onSaved?: () => void;
  onDeleted?: () => void;
}

interface Header {
  key: string;
  value: string;
}

interface AuthConfig {
  [key: string]: any;
}

interface Endpoint {
  id: number;
  http_agent_id: number;
  name: string;
  description: string;
  method: string;
  path_template: string;
  query_template: string;
  headers_json: string;
  body_template: string;
  parameters: string;
  response_template: string;
}

const AUTH_TYPES = [
  { value: 'none', label: 'Sem autenticação' },
  { value: 'api_key', label: 'API Key' },
  { value: 'bearer', label: 'Bearer Token' },
  { value: 'basic', label: 'HTTP Basic' },
  { value: 'oauth2', label: 'OAuth 2.0' },
];

const OAUTH2_GRANT_TYPES = [
  { value: 'client_credentials', label: 'Client Credentials' },
  { value: 'authorization_code', label: 'Authorization Code (requer interação)' },
];

const AVAILABLE_MODELS = [
  'gpt-4o-mini',
  'gpt-4o',
  'gpt-4-turbo',
  'gpt-3.5-turbo',
];

export function HTTPAgentEditor({ agentConfigId, onClose, onSaved, onDeleted }: HTTPAgentEditorProps) {
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  // AgentConfig fields
  const [name, setName] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [description, setDescription] = useState('');
  const [model, setModel] = useState('gpt-4o-mini');
  const [systemPrompt, setSystemPrompt] = useState('');
  const [enabled, setEnabled] = useState(true);

  // HTTPAgent fields
  const [baseURL, setBaseURL] = useState('');
  const [authType, setAuthType] = useState('none');
  const [authConfig, setAuthConfig] = useState<AuthConfig>({});
  const [defaultHeaders, setDefaultHeaders] = useState<Header[]>([]);
  const [timeoutSeconds, setTimeoutSeconds] = useState(30);
  const [retryCount, setRetryCount] = useState(3);

  // Endpoints
  const [endpoints, setEndpoints] = useState<Endpoint[]>([]);
  const [showEndpointEditor, setShowEndpointEditor] = useState(false);
  const [editingEndpoint, setEditingEndpoint] = useState<Endpoint | null>(null);

  useEffect(() => {
    console.log('HTTPAgentEditor - agentConfigId:', agentConfigId);
    if (agentConfigId) {
      loadAgent();
    } else {
      setLoading(false);
    }
  }, [agentConfigId]);

  const loadAgent = async () => {
    console.log('HTTPAgentEditor - loadAgent chamado com ID:', agentConfigId);
    setLoading(true);
    setError('');

    try {
      const agent = await GetHTTPAgentFull(agentConfigId!);
      console.log('HTTPAgentEditor - Agente carregado:', agent);

      setName(agent.name);
      setDisplayName(agent.display_name);
      setDescription(agent.description);
      setModel(agent.model || 'gpt-4o-mini');
      setSystemPrompt(agent.system_prompt || '');
      setEnabled(agent.enabled);

      setBaseURL(agent.base_url);
      setAuthType(agent.auth_type || 'none');
      setTimeoutSeconds(agent.timeout_seconds || 30);
      setRetryCount(agent.retry_count || 3);

      try {
        const parsedAuthConfig = agent.auth_config ? JSON.parse(agent.auth_config) : {};
        setAuthConfig(parsedAuthConfig);
      } catch {
        setAuthConfig({});
      }

      try {
        const headers = agent.default_headers ? JSON.parse(agent.default_headers) : {};
        const headersList = Object.entries(headers).map(([key, value]) => ({ key, value: value as string }));
        setDefaultHeaders(headersList);
      } catch {
        setDefaultHeaders([]);
      }

      // Carregar endpoints
      await loadEndpoints();
    } catch (err: any) {
      setError('Erro ao carregar agente: ' + (err.message || err));
    } finally {
      setLoading(false);
    }
  };

  const loadEndpoints = async () => {
    if (!agentConfigId) return;
    try {
      const endpointsList = await GetHTTPEndpointsByAgentID(agentConfigId);
      setEndpoints(endpointsList || []);
    } catch (err: any) {
      console.error('Erro ao carregar endpoints:', err);
    }
  };

  const handleSave = async () => {
    if (!name.trim()) {
      setError('Nome é obrigatório');
      return;
    }
    if (!displayName.trim()) {
      setError('Nome de exibição é obrigatório');
      return;
    }
    if (!baseURL.trim()) {
      setError('URL base é obrigatória');
      return;
    }

    setSaving(true);
    setError('');

    try {
      // Converte headers para JSON
      const headersObj: Record<string, string> = {};
      for (const h of defaultHeaders) {
        if (h.key.trim()) {
          headersObj[h.key.trim()] = h.value;
        }
      }
      const headersJSON = JSON.stringify(headersObj);
      const authConfigJSON = JSON.stringify(authConfig);

      if (agentConfigId) {
        // Atualiza existente
        await UpdateHTTPAgentFull(
          agentConfigId,
          displayName,
          description,
          model,
          systemPrompt,
          enabled,
          baseURL,
          authType,
          authConfigJSON,
          headersJSON,
          timeoutSeconds,
          retryCount
        );
      } else {
        // Cria novo
        await CreateHTTPAgentFull(
          name,
          displayName,
          description,
          model,
          systemPrompt,
          enabled,
          baseURL,
          authType,
          authConfigJSON,
          headersJSON,
          timeoutSeconds,
          retryCount
        );
      }

      onSaved?.();
      onClose?.();
    } catch (err: any) {
      setError('Erro ao salvar: ' + (err.message || err));
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async () => {
    if (!window.confirm('Tem certeza que deseja excluir este agente HTTP e todos os seus endpoints?')) {
      return;
    }

    try {
      await DeleteHTTPAgentFull(agentConfigId!);
      onDeleted?.();
      onClose?.();
    } catch (err: any) {
      setError('Erro ao excluir: ' + (err.message || err));
    }
  };

  const addHeader = () => {
    setDefaultHeaders([...defaultHeaders, { key: '', value: '' }]);
  };

  const removeHeader = (index: number) => {
    setDefaultHeaders(defaultHeaders.filter((_, i) => i !== index));
  };

  const updateHeader = (index: number, field: 'key' | 'value', value: string) => {
    const updated = [...defaultHeaders];
    updated[index][field] = value;
    setDefaultHeaders(updated);
  };

  // Funções de gerenciamento de endpoints
  const handleNewEndpoint = () => {
    if (!agentConfigId) {
      alert('Salve o agente HTTP primeiro antes de adicionar endpoints.');
      return;
    }
    setEditingEndpoint(null);
    setShowEndpointEditor(true);
  };

  const handleEditEndpoint = (endpoint: Endpoint) => {
    setEditingEndpoint(endpoint);
    setShowEndpointEditor(true);
  };

  const handleDeleteEndpoint = async (endpointId: number) => {
    if (!window.confirm('Tem certeza que deseja excluir este endpoint?')) {
      return;
    }

    try {
      await DeleteHTTPEndpoint(endpointId);
      await loadEndpoints();
    } catch (err: any) {
      setError('Erro ao excluir endpoint: ' + (err.message || err));
    }
  };

  const handleSaveEndpoint = async (endpointData: any) => {
    try {
      if (endpointData.id) {
        // Atualizar
        await UpdateHTTPEndpoint(
          endpointData.id,
          endpointData.name,
          endpointData.description,
          endpointData.method,
          endpointData.pathTemplate,
          endpointData.queryTemplate,
          endpointData.headersJSON,
          endpointData.bodyTemplate,
          endpointData.parametersJSON,
          endpointData.responseTemplate
        );
      } else {
        // Criar novo
        await CreateHTTPEndpoint(
          agentConfigId!,
          endpointData.name,
          endpointData.description,
          endpointData.method,
          endpointData.pathTemplate,
          endpointData.queryTemplate,
          endpointData.headersJSON,
          endpointData.bodyTemplate,
          endpointData.parametersJSON,
          endpointData.responseTemplate
        );
      }
      
      setShowEndpointEditor(false);
      setEditingEndpoint(null);
      await loadEndpoints();
    } catch (err: any) {
      throw new Error('Erro ao salvar endpoint: ' + (err.message || err));
    }
  };

  const handleTestEndpoint = async (endpointData: any) => {
    try {
      // TestHTTPEndpoint precisa de 3 argumentos: id (number), params (string JSON), httpAgentId (string)
      const result = await TestHTTPEndpoint(endpointData.id, '{}', String(agentConfigId!));
      alert('Teste executado com sucesso!\n\n' + result);
    } catch (err: any) {
      alert('Erro ao testar endpoint:\n\n' + (err.message || err));
    }
  };

  const getMethodColor = (method: string) => {
    const colors: Record<string, string> = {
      GET: '#61affe',
      POST: '#49cc90',
      PUT: '#fca130',
      PATCH: '#50e3c2',
      DELETE: '#f93e3e',
    };
    return colors[method] || '#999';
  };

  if (loading) {
    return (
      <div className="http-agent-editor">
        <div className="loading-message">Carregando agente HTTP...</div>
      </div>
    );
  }

  return (
    <div className="http-agent-editor">
      <div className="editor-header">
        <h2>{agentConfigId ? 'Editar Agente HTTP' : 'Novo Agente HTTP'}</h2>
        <button className="close-button" onClick={onClose} aria-label="Fechar">×</button>
      </div>

      {error && <div className="error-message">{error}</div>}

      <div className="editor-content">
        {/* Informações Básicas */}
        <section className="editor-section">
          <h3>Informações Básicas</h3>
          
          <div className="form-row">
            <div className="form-group">
              <label htmlFor="agent-name">Nome interno *</label>
              <Input
                id="agent-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="http_agent_exemplo"
                disabled={!!agentConfigId}
              />
            </div>

            <div className="form-group">
              <label htmlFor="agent-display-name">Nome de exibição *</label>
              <Input
                id="agent-display-name"
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
                placeholder="Meu Agente HTTP"
              />
            </div>
          </div>

          <div className="form-group">
            <label htmlFor="agent-description">Descrição</label>
            <Textarea
              id="agent-description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Descrição do agente"
              rows={2}
            />
          </div>

          <div className="form-row">
            <div className="form-group">
              <label htmlFor="agent-model">Modelo</label>
              <Select
                id="agent-model"
                value={model}
                onChange={(e) => setModel(e.target.value)}
                options={AVAILABLE_MODELS.map(m => ({ value: m, label: m }))}
              />
            </div>

            <div className="form-group checkbox-group">
              <Checkbox
                id="agent-enabled"
                checked={enabled}
                onChange={(e) => setEnabled(e.target.checked)}
                label="Agente ativo"
              />
            </div>
          </div>

          <div className="form-group">
            <label htmlFor="agent-system-prompt">Prompt do sistema</label>
            <Textarea
              id="agent-system-prompt"
              value={systemPrompt}
              onChange={(e) => setSystemPrompt(e.target.value)}
              placeholder="Instruções para o agente..."
              rows={4}
            />
          </div>
        </section>

        {/* Configuração HTTP */}
        <section className="editor-section">
          <h3>Configuração HTTP</h3>

          <div className="form-group">
            <label htmlFor="base-url">URL Base *</label>
            <Input
              id="base-url"
              value={baseURL}
              onChange={(e) => setBaseURL(e.target.value)}
              placeholder="https://api.exemplo.com"
            />
          </div>

          <div className="form-row">
            <div className="form-group">
              <label htmlFor="timeout">Timeout (segundos)</label>
              <Input
                id="timeout"
                type="number"
                value={timeoutSeconds.toString()}
                onChange={(e) => setTimeoutSeconds(parseInt(e.target.value) || 30)}
                min="1"
                max="300"
              />
            </div>

            <div className="form-group">
              <label htmlFor="retry-count">Tentativas de retry</label>
              <Input
                id="retry-count"
                type="number"
                value={retryCount.toString()}
                onChange={(e) => setRetryCount(parseInt(e.target.value) || 3)}
                min="0"
                max="10"
              />
            </div>
          </div>
        </section>

        {/* Autenticação */}
        <section className="editor-section">
          <h3>Autenticação</h3>

          <div className="form-group">
            <label htmlFor="auth-type">Tipo de autenticação</label>
            <Select
              id="auth-type"
              value={authType}
              onChange={(e) => setAuthType(e.target.value)}
              options={AUTH_TYPES}
            />
          </div>

          {authType === 'api_key' && (
            <>
              <div className="form-group">
                <label htmlFor="api-key-header">Nome do header</label>
                <Input
                  id="api-key-header"
                  value={authConfig.header_name || 'X-API-Key'}
                  onChange={(e) => setAuthConfig({ ...authConfig, header_name: e.target.value })}
                  placeholder="X-API-Key"
                />
              </div>
              <div className="form-group">
                <label htmlFor="api-key-value">Valor da chave</label>
                <Input
                  id="api-key-value"
                  type="password"
                  value={authConfig.api_key || ''}
                  onChange={(e) => setAuthConfig({ ...authConfig, api_key: e.target.value })}
                  placeholder="sua-api-key"
                />
              </div>
            </>
          )}

          {authType === 'bearer' && (
            <div className="form-group">
              <label htmlFor="bearer-token">Token Bearer</label>
              <Input
                id="bearer-token"
                type="password"
                value={authConfig.token || ''}
                onChange={(e) => setAuthConfig({ ...authConfig, token: e.target.value })}
                placeholder="seu-token"
              />
            </div>
          )}

          {authType === 'basic' && (
            <>
              <div className="form-group">
                <label htmlFor="basic-username">Usuário</label>
                <Input
                  id="basic-username"
                  value={authConfig.username || ''}
                  onChange={(e) => setAuthConfig({ ...authConfig, username: e.target.value })}
                  placeholder="usuário"
                />
              </div>
              <div className="form-group">
                <label htmlFor="basic-password">Senha</label>
                <Input
                  id="basic-password"
                  type="password"
                  value={authConfig.password || ''}
                  onChange={(e) => setAuthConfig({ ...authConfig, password: e.target.value })}
                  placeholder="senha"
                />
              </div>
            </>
          )}

          {authType === 'oauth2' && (
            <>
              <div className="form-group">
                <label htmlFor="oauth-grant-type">Grant Type</label>
                <Select
                  id="oauth-grant-type"
                  value={authConfig.grant_type || 'client_credentials'}
                  onChange={(e) => setAuthConfig({ ...authConfig, grant_type: e.target.value })}
                  options={OAUTH2_GRANT_TYPES}
                />
              </div>
              <div className="form-group">
                <label htmlFor="oauth-token-url">Token URL</label>
                <Input
                  id="oauth-token-url"
                  value={authConfig.token_url || ''}
                  onChange={(e) => setAuthConfig({ ...authConfig, token_url: e.target.value })}
                  placeholder="https://oauth.exemplo.com/token"
                />
              </div>
              <div className="form-row">
                <div className="form-group">
                  <label htmlFor="oauth-client-id">Client ID</label>
                  <Input
                    id="oauth-client-id"
                    value={authConfig.client_id || ''}
                    onChange={(e) => setAuthConfig({ ...authConfig, client_id: e.target.value })}
                    placeholder="client-id"
                  />
                </div>
                <div className="form-group">
                  <label htmlFor="oauth-client-secret">Client Secret</label>
                  <Input
                    id="oauth-client-secret"
                    type="password"
                    value={authConfig.client_secret || ''}
                    onChange={(e) => setAuthConfig({ ...authConfig, client_secret: e.target.value })}
                    placeholder="client-secret"
                  />
                </div>
              </div>
            </>
          )}
        </section>

        {/* Headers Padrão */}
        <section className="editor-section">
          <div className="section-header">
            <h3>Headers Padrão</h3>
            <Button onClick={addHeader} variant="secondary" size="sm">
              + Adicionar header
            </Button>
          </div>

          <div className="headers-list">
            {defaultHeaders.map((header, index) => (
              <div key={index} className="header-row">
                <Input
                  value={header.key}
                  onChange={(e) => updateHeader(index, 'key', e.target.value)}
                  placeholder="Nome do header"
                />
                <Input
                  value={header.value}
                  onChange={(e) => updateHeader(index, 'value', e.target.value)}
                  placeholder="Valor"
                />
                <button
                  type="button"
                  className="remove-button"
                  onClick={() => removeHeader(index)}
                  aria-label="Remover header"
                >
                  ×
                </button>
              </div>
            ))}
            {defaultHeaders.length === 0 && (
              <div className="empty-message">Nenhum header configurado</div>
            )}
          </div>
        </section>

        {/* Endpoints */}
        {agentConfigId && (
          <section className="editor-section">
            <div className="section-header">
              <h3>Endpoints</h3>
              <Button onClick={handleNewEndpoint} variant="secondary" size="sm">
                + Adicionar endpoint
              </Button>
            </div>

            <div className="endpoints-list">
              {endpoints.length === 0 ? (
                <div className="empty-message">
                  Nenhum endpoint configurado. Adicione o primeiro para que o agente possa fazer chamadas à API.
                </div>
              ) : (
                <div className="endpoints-table">
                  <div className="endpoints-table-header">
                    <div className="col-method">Método</div>
                    <div className="col-name">Nome</div>
                    <div className="col-path">Path</div>
                    <div className="col-actions">Ações</div>
                  </div>
                  {endpoints.map((endpoint) => (
                    <div key={endpoint.id} className="endpoints-table-row">
                      <div className="col-method">
                        <span 
                          className="method-badge" 
                          style={{ backgroundColor: getMethodColor(endpoint.method) }}
                        >
                          {endpoint.method}
                        </span>
                      </div>
                      <div className="col-name">{endpoint.name}</div>
                      <div className="col-path">{endpoint.path_template}</div>
                      <div className="col-actions">
                        <button
                          onClick={() => handleEditEndpoint(endpoint)}
                          className="action-button edit"
                          title="Editar"
                        >
                          ✏️
                        </button>
                        <button
                          onClick={() => handleDeleteEndpoint(endpoint.id)}
                          className="action-button delete"
                          title="Excluir"
                        >
                          🗑️
                        </button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </section>
        )}
      </div>

      {/* Ações */}
      <div className="editor-actions">
        <div className="actions-left">
          {agentConfigId && (
            <Button onClick={handleDelete} variant="danger">
              Excluir agente
            </Button>
          )}
        </div>
        <div className="actions-right">
          <Button onClick={onClose} variant="secondary">
            Cancelar
          </Button>
          <Button onClick={handleSave} disabled={saving}>
            {saving ? 'Salvando...' : agentConfigId ? 'Salvar' : 'Criar'}
          </Button>
        </div>
      </div>

      {/* Modal do EndpointEditor */}
      {showEndpointEditor && (
        <Modal
          id="endpoint-editor-modal"
          title={editingEndpoint ? 'Editar Endpoint' : 'Novo Endpoint'}
          onClose={() => {
            setShowEndpointEditor(false);
            setEditingEndpoint(null);
          }}
          size="xl"
        >
          <EndpointEditor
            endpoint={editingEndpoint}
            agentId={agentConfigId || undefined}
            onSave={handleSaveEndpoint}
            onCancel={() => {
              setShowEndpointEditor(false);
              setEditingEndpoint(null);
            }}
            onTest={editingEndpoint ? handleTestEndpoint : undefined}
          />
        </Modal>
      )}
    </div>
  );
}
