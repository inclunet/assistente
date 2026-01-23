import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { 
  GetAllAgentConfigs, 
  SaveOrUpdateAgentConfig, 
  DeleteAgentConfig,
  TestAgent 
} from '../../wailsjs/go/main/App';
import './AgentsPage.css';

interface Agent {
  id: number;
  name: string;
  type: 'http' | 'file' | 'mcp' | 'image';
  enabled: boolean;
  config: any;
}

export default function AgentsPage() {
  const { t } = useTranslation();
  const [agents, setAgents] = useState<Agent[]>([]);
  const [loading, setLoading] = useState(true);
  const [filterType, setFilterType] = useState<string>('all');
  const [showModal, setShowModal] = useState(false);
  const [editingAgent, setEditingAgent] = useState<Agent | null>(null);

  useEffect(() => {
    loadAgents();
  }, []);

  const loadAgents = async () => {
    setLoading(true);
    try {
      const result = await GetAllAgentConfigs();
      const mapped = result.map((cfg: any) => ({
        id: cfg.id,
        name: cfg.agent_name,
        type: cfg.agent_type,
        enabled: cfg.enabled,
        config: cfg.config || '{}'
      }));
      setAgents(mapped || []);
    } catch (error) {
      console.error('Erro ao carregar agentes:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleCreateOrUpdate = async (agent: Agent) => {
    try {
      await SaveOrUpdateAgentConfig(
        agent.name,
        agent.type,
        typeof agent.config === 'string' ? agent.config : JSON.stringify(agent.config),
        '',  // description
        '',  // prompt_template
        '',  // response_template
        '',  // error_template
        agent.enabled
      );
      await loadAgents();
      setShowModal(false);
      setEditingAgent(null);
    } catch (error) {
      console.error('Erro ao salvar agente:', error);
    }
  };

  const handleDelete = async (id: number) => {
    if (!confirm(t('agents.confirmDelete'))) return;
    
    try {
      await DeleteAgentConfig(id);
      setAgents(prev => prev.filter(a => a.id !== id));
    } catch (error) {
      console.error('Erro ao deletar agente:', error);
    }
  };

  const handleTest = async (agent: Agent) => {
    try {
      await TestAgent(agent.name, agent.type);
      alert(t('agents.testSuccess', 'Agente testado com sucesso!'));
    } catch (error) {
      console.error('Erro ao testar agente:', error);
      alert(t('agents.testError', 'Erro ao testar agente'));
    }
  };

  const filteredAgents = agents.filter(agent => 
    filterType === 'all' || agent.type === filterType
  );

  const agentTypeLabels: Record<string, string> = {
    http: 'HTTP Agent',
    file: 'File Agent',
    mcp: 'MCP Agent',
    image: 'Image Agent',
  };

  return (
    <div className="agents-page">
      <header className="agents-header">
        <h1>{t('agents.title', 'Gerenciar Agentes')}</h1>
        <div className="header-actions">
          <select value={filterType} onChange={(e) => setFilterType(e.target.value)}>
            <option value="all">Todos os tipos</option>
            <option value="http">HTTP</option>
            <option value="file">File</option>
            <option value="mcp">MCP</option>
            <option value="image">Image</option>
          </select>
          <button onClick={() => { setEditingAgent(null); setShowModal(true); }} className="btn-primary">
            {t('agents.new', 'Novo Agente')}
          </button>
        </div>
      </header>

      {loading ? (
        <div className="loading">Carregando agentes...</div>
      ) : (
        <div className="agents-grid">
          {filteredAgents.map(agent => (
            <div key={agent.id} className={`agent-card ${agent.enabled ? '' : 'disabled'}`}>
              <div className="agent-content">
                <div className="agent-header-card">
                  <h3>{agent.name}</h3>
                  <span className={`type-badge ${agent.type}`}>{agentTypeLabels[agent.type]}</span>
                </div>
                <p className="agent-status">
                  Status: {agent.enabled ? '✅ Ativo' : '❌ Inativo'}
                </p>
              </div>
              <div className="agent-actions">
                <button onClick={() => handleTest(agent)} title="Testar agente">🧪</button>
                <button onClick={() => { setEditingAgent(agent); setShowModal(true); }} title="Editar">✏️</button>
                <button onClick={() => handleDelete(agent.id)} className="delete-btn" title="Deletar">🗑️</button>
              </div>
            </div>
          ))}
          {filteredAgents.length === 0 && (
            <div className="empty-state">
              <p>{t('agents.empty', 'Nenhum agente encontrado')}</p>
            </div>
          )}
        </div>
      )}

      {showModal && (
        <AgentModal
          agent={editingAgent}
          onSave={handleCreateOrUpdate}
          onClose={() => { setShowModal(false); setEditingAgent(null); }}
        />
      )}
    </div>
  );
}

interface AgentModalProps {
  agent: Agent | null;
  onSave: (agent: Agent) => void;
  onClose: () => void;
}

function AgentModal({ agent, onSave, onClose }: AgentModalProps) {
  const [name, setName] = useState(agent?.name || '');
  const [type, setType] = useState<Agent['type']>(agent?.type || 'http');
  const [enabled, setEnabled] = useState(agent?.enabled ?? true);
  const [config, setConfig] = useState(JSON.stringify(agent?.config || {}, null, 2));

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    try {
      const parsedConfig = JSON.parse(config);
      onSave({
        id: agent?.id || 0,
        name,
        type,
        enabled,
        config: parsedConfig,
      });
    } catch (error) {
      alert('Erro ao parsear configuração JSON');
    }
  };

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal-content agent-modal" onClick={e => e.stopPropagation()}>
        <h2>{agent ? 'Editar Agente' : 'Novo Agente'}</h2>
        <form onSubmit={handleSubmit}>
          <div className="form-group">
            <label>Nome*</label>
            <input
              type="text"
              value={name}
              onChange={e => setName(e.target.value)}
              required
            />
          </div>
          <div className="form-group">
            <label>Tipo*</label>
            <select value={type} onChange={e => setType(e.target.value as Agent['type'])} required>
              <option value="http">HTTP Agent</option>
              <option value="file">File Agent</option>
              <option value="mcp">MCP Agent</option>
              <option value="image">Image Agent</option>
            </select>
          </div>
          <div className="form-group checkbox-group">
            <label>
              <input
                type="checkbox"
                checked={enabled}
                onChange={e => setEnabled(e.target.checked)}
              />
              Ativo
            </label>
          </div>
          <div className="form-group">
            <label>Configuração (JSON)*</label>
            <textarea
              value={config}
              onChange={e => setConfig(e.target.value)}
              rows={12}
              className="config-textarea"
              placeholder='{"url": "https://api.example.com", ...}'
              required
            />
          </div>
          <div className="modal-actions">
            <button type="button" onClick={onClose} className="btn-secondary">
              Cancelar
            </button>
            <button type="submit" className="btn-primary">
              Salvar
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
