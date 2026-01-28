import { useState, useEffect } from 'react';
import {
  GetMCPAgent,
  GetAgentConfigByID,
  CreateMCPAgentFull,
  UpdateMCPAgentFull,
  ConnectMCPAgent,
  DisconnectMCPAgent,
  GetMCPAgentStatus,
  TestMCPAgent,
  DeleteMCPAgentFull,
} from '../../../wailsjs/go/main/App';
import { Input } from '../ui/Input';
import { Textarea } from '../ui/Textarea';
import { Select } from '../ui/Select';
import { Checkbox } from '../ui/Checkbox';
import { Button } from '../ui/Button';
import './MCPAgentEditor.css';

interface MCPAgentEditorProps {
  mcpAgentId?: number | null;
  onClose?: () => void;
  onSaved?: () => void;
  onDeleted?: () => void;
}

interface ServerInfo {
  name?: string;
  version?: string;
  protocolVersion?: string;
  capabilities?: any;
}

interface Tool {
  name: string;
  description?: string;
}

const TRANSPORT_TYPES = [
  { value: 'stdio', label: 'Local (stdio)', description: 'Servidor MCP local executado como processo filho.' },
  { value: 'http', label: 'Remoto (HTTP/SSE)', description: 'Servidor MCP remoto via HTTP com Server-Sent Events.' }
];

const AUTH_TYPES = [
  { value: 'none', label: 'Nenhuma' },
  { value: 'bearer', label: 'Bearer Token' },
  { value: 'api_key', label: 'API Key' }
];

const EXECUTION_MODES = [
  { value: 'convert', label: 'Converter (Padrão)', description: 'Converte tools MCP para formato OpenAI. Compatível com qualquer modelo.' },
  { value: 'native', label: 'Nativo', description: 'Passa tools MCP diretamente. Para modelos com suporte nativo (ex: Claude).' },
  { value: 'passthrough', label: 'Passthrough', description: 'Envia tarefa direto ao servidor MCP. Útil quando o servidor já tem um LLM.' }
];

const AVAILABLE_MODELS = [
  'gpt-4o-mini',
  'gpt-4o',
  'gpt-4-turbo',
  'gpt-3.5-turbo',
  'claude-3-5-sonnet-20241022',
  'claude-3-haiku-20240307'
];

export function MCPAgentEditor({ mcpAgentId, onClose, onSaved, onDeleted }: MCPAgentEditorProps) {
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [successMessage, setSuccessMessage] = useState('');

  // Form data
  const [name, setName] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [description, setDescription] = useState('');
  const [model, setModel] = useState('gpt-4o-mini');
  const [systemPrompt, setSystemPrompt] = useState('');
  const [enabled, setEnabled] = useState(true);

  // Transport configuration
  const [transportType, setTransportType] = useState('stdio');

  // Stdio configuration
  const [serverCommand, setServerCommand] = useState('');
  const [serverArgs, setServerArgs] = useState('[]');
  const [serverEnv, setServerEnv] = useState('[]');
  const [workingDir, setWorkingDir] = useState('');

  // HTTP configuration
  const [serverURL, setServerURL] = useState('');
  const [authType, setAuthType] = useState('none');
  const [authValue, setAuthValue] = useState('');
  const [httpHeaders, setHttpHeaders] = useState('{}');

  // Common configuration
  const [executionMode, setExecutionMode] = useState('convert');
  const [autoConnect, setAutoConnect] = useState(false);

  // Connection status
  const [connected, setConnected] = useState(false);
  const [connecting, setConnecting] = useState(false);
  const [serverInfo, setServerInfo] = useState<ServerInfo | null>(null);
  const [availableTools, setAvailableTools] = useState<Tool[]>([]);

  // Testing
  const [testing, setTesting] = useState(false);
  const [testTask, setTestTask] = useState('');
  const [testResult, setTestResult] = useState('');

  useEffect(() => {
    console.log('MCPAgentEditor - mcpAgentId:', mcpAgentId);
    if (mcpAgentId) {
      loadMCPAgent();
    } else {
      setLoading(false);
    }
  }, [mcpAgentId]);

  const loadMCPAgent = async () => {
    console.log('MCPAgentEditor - loadMCPAgent chamado com ID:', mcpAgentId);
    setLoading(true);
    setError('');

    try {
      const mcpAgent = await GetMCPAgent(mcpAgentId!);
      console.log('MCPAgentEditor - MCP Agent carregado:', mcpAgent);
      const agentConfig = await GetAgentConfigByID(mcpAgent.agent_config_id);
      console.log('MCPAgentEditor - Agent Config carregado:', agentConfig);

      setName(agentConfig.name);
      setDisplayName(agentConfig.display_name);
      setDescription(agentConfig.description);
      setModel(agentConfig.model || 'gpt-4o-mini');
      setSystemPrompt(agentConfig.system_prompt || '');
      setEnabled(agentConfig.enabled);

      setTransportType(mcpAgent.transport_type || 'stdio');
      setServerCommand(mcpAgent.server_command || '');
      setServerArgs(mcpAgent.server_args || '[]');
      setServerEnv(mcpAgent.server_env || '[]');
      setWorkingDir(mcpAgent.working_dir || '');
      setServerURL(mcpAgent.server_url || '');
      setAuthType(mcpAgent.auth_type || 'none');
      setAuthValue(mcpAgent.auth_value || '');
      setHttpHeaders(mcpAgent.http_headers || '{}');
      setExecutionMode(mcpAgent.execution_mode || 'convert');
      setAutoConnect(mcpAgent.auto_connect || false);

      // Check connection status
      await refreshConnectionStatus();
    } catch (err: any) {
      setError('Erro ao carregar MCP Agent: ' + (err.message || err));
    } finally {
      setLoading(false);
    }
  };

  const refreshConnectionStatus = async () => {
    if (!mcpAgentId) return;

    try {
      const status = await GetMCPAgentStatus(mcpAgentId);
      setConnected(status.connected);
      setServerInfo(status.server_info || null);
      setAvailableTools(status.tools || []);
    } catch {
      setConnected(false);
      setServerInfo(null);
      setAvailableTools([]);
    }
  };

  const handleConnect = async () => {
    setConnecting(true);
    setError('');

    try {
      await ConnectMCPAgent(mcpAgentId!);
      await refreshConnectionStatus();
      setSuccessMessage('Conectado ao servidor MCP!');
      setTimeout(() => setSuccessMessage(''), 3000);
    } catch (err: any) {
      setError('Erro ao conectar: ' + (err.message || err));
    } finally {
      setConnecting(false);
    }
  };

  const handleDisconnect = async () => {
    setConnecting(true);
    setError('');

    try {
      await DisconnectMCPAgent(mcpAgentId!);
      setConnected(false);
      setServerInfo(null);
      setAvailableTools([]);
      setSuccessMessage('Desconectado do servidor MCP');
      setTimeout(() => setSuccessMessage(''), 3000);
    } catch (err: any) {
      setError('Erro ao desconectar: ' + (err.message || err));
    } finally {
      setConnecting(false);
    }
  };

  const handleTest = async () => {
    if (!testTask.trim()) {
      setError('Digite uma tarefa para testar');
      return;
    }

    setTesting(true);
    setTestResult('');
    setError('');

    try {
      const result = await TestMCPAgent(mcpAgentId!, testTask);
      setTestResult(result);
    } catch (err: any) {
      setError('Erro ao testar: ' + (err.message || err));
    } finally {
      setTesting(false);
    }
  };

  const handleSubmit = async () => {
    if (!name.trim()) {
      setError('Nome é obrigatório');
      return;
    }

    if (transportType === 'stdio' && !serverCommand.trim()) {
      setError('Comando do servidor é obrigatório para transporte local');
      return;
    }

    if (transportType === 'http' && !serverURL.trim()) {
      setError('URL do servidor é obrigatória para transporte remoto');
      return;
    }

    setSaving(true);
    setError('');

    try {
      if (mcpAgentId) {
        // Atualiza existente
        await UpdateMCPAgentFull(
          mcpAgentId,
          displayName,
          description,
          model,
          systemPrompt,
          transportType,
          serverCommand,
          serverArgs,
          serverEnv,
          workingDir,
          serverURL,
          authType,
          authValue,
          httpHeaders,
          executionMode,
          autoConnect,
          enabled
        );
      } else {
        // Cria novo
        await CreateMCPAgentFull(
          name,
          displayName,
          description,
          model,
          systemPrompt,
          transportType,
          serverCommand,
          serverArgs,
          serverEnv,
          workingDir,
          serverURL,
          authType,
          authValue,
          httpHeaders,
          executionMode,
          autoConnect,
          enabled
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
    if (!window.confirm('Tem certeza que deseja excluir este agente MCP?')) {
      return;
    }

    try {
      await DeleteMCPAgentFull(mcpAgentId!);
      onDeleted?.();
      onClose?.();
    } catch (err: any) {
      setError('Erro ao excluir: ' + (err.message || err));
    }
  };

  if (loading) {
    return (
      <div className="mcp-agent-editor">
        <div className="loading-message">Carregando agente MCP...</div>
      </div>
    );
  }

  return (
    <div className="mcp-agent-editor">
      <div className="editor-header">
        <h2>{mcpAgentId ? 'Editar Agente MCP' : 'Novo Agente MCP'}</h2>
        <button className="close-button" onClick={onClose} aria-label="Fechar">×</button>
      </div>

      {error && <div className="error-message">{error}</div>}
      {successMessage && <div className="success-message">{successMessage}</div>}

      <div className="editor-content">
        {/* Informações Básicas */}
        <section className="editor-section">
          <h3>Informações Básicas</h3>

          <div className="form-row">
            <div className="form-group">
              <label htmlFor="mcp-name">Nome interno *</label>
              <Input
                id="mcp-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="mcp_agent_exemplo"
                disabled={!!mcpAgentId}
              />
            </div>

            <div className="form-group">
              <label htmlFor="mcp-display-name">Nome de exibição</label>
              <Input
                id="mcp-display-name"
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
                placeholder="Meu Agente MCP"
              />
            </div>
          </div>

          <div className="form-group">
            <label htmlFor="mcp-description">Descrição</label>
            <Textarea
              id="mcp-description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Descrição do agente"
              rows={2}
            />
          </div>

          <div className="form-row">
            <div className="form-group">
              <label htmlFor="mcp-model">Modelo</label>
              <Select
                id="mcp-model"
                value={model}
                onChange={(e) => setModel(e.target.value)}
                options={AVAILABLE_MODELS.map(m => ({ value: m, label: m }))}
              />
            </div>

            <div className="form-group checkbox-group">
              <Checkbox
                id="mcp-enabled"
                checked={enabled}
                onChange={(e) => setEnabled(e.target.checked)}
                label="Agente ativo"
              />
            </div>
          </div>

          <div className="form-group">
            <label htmlFor="mcp-system-prompt">Prompt do sistema</label>
            <Textarea
              id="mcp-system-prompt"
              value={systemPrompt}
              onChange={(e) => setSystemPrompt(e.target.value)}
              placeholder="Instruções para o agente..."
              rows={4}
            />
          </div>
        </section>

        {/* Tipo de Transporte */}
        <section className="editor-section">
          <h3>Tipo de Transporte</h3>

          <div className="transport-options">
            {TRANSPORT_TYPES.map(type => (
              <label key={type.value} className="radio-card">
                <input
                  type="radio"
                  name="transport-type"
                  value={type.value}
                  checked={transportType === type.value}
                  onChange={(e) => setTransportType(e.target.value)}
                />
                <div className="radio-card-content">
                  <div className="radio-card-title">{type.label}</div>
                  <div className="radio-card-description">{type.description}</div>
                </div>
              </label>
            ))}
          </div>
        </section>

        {/* Configuração Stdio */}
        {transportType === 'stdio' && (
          <section className="editor-section">
            <h3>Configuração Local (stdio)</h3>

            <div className="form-group">
              <label htmlFor="server-command">Comando do servidor *</label>
              <Input
                id="server-command"
                value={serverCommand}
                onChange={(e) => setServerCommand(e.target.value)}
                placeholder="node server.js"
              />
            </div>

            <div className="form-group">
              <label htmlFor="server-args">Argumentos (JSON array)</label>
              <Textarea
                id="server-args"
                value={serverArgs}
                onChange={(e) => setServerArgs(e.target.value)}
                placeholder='["--port", "3000"]'
                rows={3}
              />
            </div>

            <div className="form-group">
              <label htmlFor="server-env">Variáveis de ambiente (JSON object)</label>
              <Textarea
                id="server-env"
                value={serverEnv}
                onChange={(e) => setServerEnv(e.target.value)}
                placeholder='{"API_KEY": "your-key"}'
                rows={3}
              />
            </div>

            <div className="form-group">
              <label htmlFor="working-dir">Diretório de trabalho</label>
              <Input
                id="working-dir"
                value={workingDir}
                onChange={(e) => setWorkingDir(e.target.value)}
                placeholder="/caminho/para/servidor"
              />
            </div>
          </section>
        )}

        {/* Configuração HTTP */}
        {transportType === 'http' && (
          <section className="editor-section">
            <h3>Configuração Remota (HTTP/SSE)</h3>

            <div className="form-group">
              <label htmlFor="server-url">URL do servidor *</label>
              <Input
                id="server-url"
                value={serverURL}
                onChange={(e) => setServerURL(e.target.value)}
                placeholder="https://mcp-server.exemplo.com"
              />
            </div>

            <div className="form-group">
              <label htmlFor="auth-type">Autenticação</label>
              <Select
                id="auth-type"
                value={authType}
                onChange={(e) => setAuthType(e.target.value)}
                options={AUTH_TYPES}
              />
            </div>

            {authType !== 'none' && (
              <div className="form-group">
                <label htmlFor="auth-value">Valor de autenticação</label>
                <Input
                  id="auth-value"
                  type="password"
                  value={authValue}
                  onChange={(e) => setAuthValue(e.target.value)}
                  placeholder="Token ou API Key"
                />
              </div>
            )}

            <div className="form-group">
              <label htmlFor="http-headers">Headers adicionais (JSON object)</label>
              <Textarea
                id="http-headers"
                value={httpHeaders}
                onChange={(e) => setHttpHeaders(e.target.value)}
                placeholder='{"X-Custom-Header": "value"}'
                rows={3}
              />
            </div>
          </section>
        )}

        {/* Modo de Execução */}
        <section className="editor-section">
          <h3>Modo de Execução</h3>

          <div className="execution-modes">
            {EXECUTION_MODES.map(mode => (
              <label key={mode.value} className="radio-card">
                <input
                  type="radio"
                  name="execution-mode"
                  value={mode.value}
                  checked={executionMode === mode.value}
                  onChange={(e) => setExecutionMode(e.target.value)}
                />
                <div className="radio-card-content">
                  <div className="radio-card-title">{mode.label}</div>
                  <div className="radio-card-description">{mode.description}</div>
                </div>
              </label>
            ))}
          </div>

          <div className="form-group checkbox-group">
            <Checkbox
              id="auto-connect"
              checked={autoConnect}
              onChange={(e) => setAutoConnect(e.target.checked)}
              label="Conectar automaticamente ao iniciar"
            />
          </div>
        </section>

        {/* Status da Conexão */}
        {mcpAgentId && (
          <section className="editor-section">
            <h3>Status da Conexão</h3>

            <div className="connection-status">
              <div className="status-indicator">
                <span className={`status-dot ${connected ? 'connected' : 'disconnected'}`}></span>
                <span>{connected ? 'Conectado' : 'Desconectado'}</span>
              </div>

              <div className="connection-actions">
                {!connected ? (
                  <Button onClick={handleConnect} disabled={connecting}>
                    {connecting ? 'Conectando...' : 'Conectar'}
                  </Button>
                ) : (
                  <Button onClick={handleDisconnect} disabled={connecting} variant="secondary">
                    Desconectar
                  </Button>
                )}
              </div>
            </div>

            {serverInfo && (
              <div className="server-info">
                <h4>Informações do Servidor</h4>
                <dl>
                  <dt>Nome:</dt>
                  <dd>{serverInfo.name || 'N/A'}</dd>
                  <dt>Versão:</dt>
                  <dd>{serverInfo.version || 'N/A'}</dd>
                  <dt>Protocolo:</dt>
                  <dd>{serverInfo.protocolVersion || 'N/A'}</dd>
                </dl>
              </div>
            )}

            {availableTools.length > 0 && (
              <div className="available-tools">
                <h4>Tools Disponíveis ({availableTools.length})</h4>
                <ul>
                  {availableTools.map((tool, idx) => (
                    <li key={idx}>
                      <strong>{tool.name}</strong>
                      {tool.description && <span> - {tool.description}</span>}
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </section>
        )}

        {/* Playground de Testes */}
        {mcpAgentId && connected && (
          <section className="editor-section">
            <h3>Testar Agente</h3>

            <div className="form-group">
              <label htmlFor="test-task">Tarefa de teste</label>
              <Textarea
                id="test-task"
                value={testTask}
                onChange={(e) => setTestTask(e.target.value)}
                placeholder="Digite uma tarefa para testar o agente..."
                rows={3}
              />
            </div>

            <Button onClick={handleTest} disabled={testing}>
              {testing ? 'Testando...' : 'Executar teste'}
            </Button>

            {testResult && (
              <div className="test-result">
                <h4>Resultado:</h4>
                <pre>{testResult}</pre>
              </div>
            )}
          </section>
        )}
      </div>

      {/* Ações */}
      <div className="editor-actions">
        <div className="actions-left">
          {mcpAgentId && (
            <Button onClick={handleDelete} variant="danger">
              Excluir agente
            </Button>
          )}
        </div>
        <div className="actions-right">
          <Button onClick={onClose} variant="secondary">
            Cancelar
          </Button>
          <Button onClick={handleSubmit} disabled={saving}>
            {saving ? 'Salvando...' : mcpAgentId ? 'Salvar' : 'Criar'}
          </Button>
        </div>
      </div>
    </div>
  );
}
