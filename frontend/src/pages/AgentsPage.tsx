import { useState, useEffect } from 'react';
// import { useTranslation } from 'react-i18next';
import { 
  GetAllAgentConfigs, 
  GetRegisteredAgents,
  SaveOrUpdateAgentConfig, 
  // DeleteAgentConfig,
  GetAllHTTPAgentsFull,
  GetAllMCPAgentsFull
} from '../../wailsjs/go/main/App';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { Toolbar, ToolbarAction } from '../components/ui/Toolbar';
import { SimpleModal } from '../components/ui/SimpleModal';
import { Modal } from '../components/ui/Modal';
import { Input } from '../components/ui/Input';
import { Textarea } from '../components/ui/Textarea';
import { Select } from '../components/ui/Select';
import { Checkbox } from '../components/ui/Checkbox';
import { Button } from '../components/ui/Button';
import { HTTPAgentEditor } from '../components/agents/HTTPAgentEditor';
import { MCPAgentEditor } from '../components/agents/MCPAgentEditor';
import { AgentTestChat } from '../components/agents/AgentTestChat';
import { AgentDiagnostic } from '../components/agents/AgentDiagnostic';
import './AgentsPage.css';

interface Agent {
  id: number;
  name: string;
  display_name?: string;
  description?: string;
  agent_type: 'http' | 'file' | 'mcp' | 'internal';
  enabled: boolean;
  model?: string;
  system_prompt?: string;
  config?: any;
}

const AVAILABLE_MODELS = [
  'gpt-4o-mini',
  'gpt-4o',
  'gpt-4-turbo',
  'gpt-3.5-turbo',
  'claude-3-5-sonnet-20241022',
  'claude-3-haiku-20240307'
];

export default function AgentsPage() {
  // const { t } = useTranslation();
  const [agents, setAgents] = useState<Agent[]>([]);
  const [loading, setLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState('');
  const [filterType, setFilterType] = useState('all');
  const [showModal, setShowModal] = useState(false);
  const [editingAgent, setEditingAgent] = useState<Agent | null>(null);
  const [saving, setSaving] = useState(false);
  
  // HTTP Agent Editor
  const [showHTTPEditor, setShowHTTPEditor] = useState(false);
  const [editingHTTPAgentId, setEditingHTTPAgentId] = useState<number | null>(null);
  
  // MCP Agent Editor
  const [showMCPEditor, setShowMCPEditor] = useState(false);
  const [editingMCPAgentId, setEditingMCPAgentId] = useState<number | null>(null);
  
  // Agent Test Chat
  const [testChatAgent, setTestChatAgent] = useState<{name: string, type: string, displayName?: string} | null>(null);
  
  // Agent Diagnostic
  const [showDiagnostic, setShowDiagnostic] = useState(false);
  const [diagnosticAgent, setDiagnosticAgent] = useState<Agent | null>(null);
  
  // Form state
  const [formName, setFormName] = useState('');
  const [formDisplayName, setFormDisplayName] = useState('');
  const [formDescription, setFormDescription] = useState('');
  const [formModel, setFormModel] = useState('gpt-4o-mini');
  const [formSystemPrompt, setFormSystemPrompt] = useState('');
  const [formEnabled, setFormEnabled] = useState(true);
  const [formError, setFormError] = useState('');

  useEffect(() => {
    loadAgents();
  }, []);

  const loadAgents = async () => {
    setLoading(true);
    try {
      // Carrega agentes registrados (internos)
      const registered = await GetRegisteredAgents() || [];
      const configs = await GetAllAgentConfigs() || [];
      
      // Carrega agentes HTTP
      const httpAgents = await GetAllHTTPAgentsFull() || [];
      console.log('HTTP Agents carregados:', httpAgents);
      
      // Carrega agentes MCP
      const mcpAgents = await GetAllMCPAgentsFull() || [];
      console.log('MCP Agents carregados:', mcpAgents);
      
      // Combina tudo em uma lista
      const allAgents: Agent[] = [
        ...registered.map((agent: any) => {
          const savedConfig = configs.find((c: any) => c.name === agent.name);
          return {
            id: savedConfig?.id || 0,
            name: agent.name,
            display_name: savedConfig?.display_name || agent.display_name,
            description: savedConfig?.description || agent.description,
            agent_type: 'internal' as const,
            enabled: savedConfig?.enabled !== false,
            model: savedConfig?.model || agent.model || 'gpt-4o-mini',
            system_prompt: savedConfig?.system_prompt || agent.system_prompt,
          };
        }),
        ...httpAgents.map((agent: any) => {
          console.log('Mapeando HTTP Agent:', agent);
          const mappedAgent = {
            id: agent.id || 0,  // Usa agent.id diretamente, não agent.agent_config.id
            name: agent.name || 'HTTP Agent',
            display_name: agent.display_name || agent.name,
            description: agent.description,
            agent_type: 'http' as const,
            enabled: agent.enabled !== false,
            model: agent.model || 'gpt-4o-mini',
            config: agent,
          };
          console.log('Agent mapeado com ID:', mappedAgent.id);
          return mappedAgent;
        }),
        ...mcpAgents.map((agent: any) => ({
          id: agent.id || 0,  // Usa agent.id diretamente
          name: agent.name || 'MCP Agent',
          display_name: agent.display_name || agent.name,
          description: agent.server_command,
          agent_type: 'mcp' as const,
          enabled: agent.enabled !== false,
          model: agent.model || 'gpt-4o-mini',
          config: agent,
        })),
      ];
      
      setAgents(allAgents);
    } catch (error) {
      console.error('Erro ao carregar agentes:', error);
    } finally {
      setLoading(false);
    }
  };

  const openEditForm = (agent: Agent) => {
    console.log('openEditForm chamado para agente:', agent.name, 'tipo:', agent.agent_type);
    console.log('Dados completos do agente:', agent);
    console.log('Agent ID que será passado:', agent.id);
    
    // Se for HTTP ou MCP, abre o editor específico
    if (agent.agent_type === 'http') {
      console.log('Abrindo HTTP Editor com ID:', agent.id);
      if (!agent.id || agent.id === 0) {
        console.error('ERRO: ID do agente HTTP é 0 ou undefined!');
        alert('Erro: ID do agente inválido. Verifique o console.');
        return;
      }
      setEditingHTTPAgentId(agent.id);
      setShowHTTPEditor(true);
      return;
    }
    
    if (agent.agent_type === 'mcp') {
      console.log('Abrindo MCP Editor com ID:', agent.id);
      if (!agent.id || agent.id === 0) {
        console.error('ERRO: ID do agente MCP é 0 ou undefined!');
        alert('Erro: ID do agente inválido. Verifique o console.');
        return;
      }
      setEditingMCPAgentId(agent.id);
      setShowMCPEditor(true);
      return;
    }
    
    // Para agentes internos, usa o formulário simples
    setEditingAgent(agent);
    setFormName(agent.name);
    setFormDisplayName(agent.display_name || '');
    setFormDescription(agent.description || '');
    setFormModel(agent.model || 'gpt-4o-mini');
    setFormSystemPrompt(agent.system_prompt || '');
    setFormEnabled(agent.enabled);
    setFormError('');
    setShowModal(true);
  };
  
  const openNewHTTPAgent = () => {
    setEditingHTTPAgentId(null);
    setShowHTTPEditor(true);
  };
  
  const openNewMCPAgent = () => {
    setEditingMCPAgentId(null);
    setShowMCPEditor(true);
  };

  const handleSave = async () => {
    if (!formDisplayName.trim()) {
      setFormError('Nome de exibição é obrigatório');
      return;
    }

    setSaving(true);
    setFormError('');

    try {
      await SaveOrUpdateAgentConfig(
        editingAgent?.name || formName,
        editingAgent?.agent_type || 'internal',
        JSON.stringify(editingAgent?.config || {}),
        formDescription,
        formSystemPrompt,
        '',  // response_template
        '',  // error_template
        formEnabled
      );
      
      await loadAgents();
      setShowModal(false);
    } catch (error: any) {
      setFormError('Erro ao salvar: ' + (error.message || error));
    } finally {
      setSaving(false);
    }
  };

  // const handleDelete = async (agent: Agent) => {
  //   if (!confirm(t('agents.confirmDelete', 'Tem certeza que deseja excluir este agente?'))) return;
  //   
  //   try {
  //     await DeleteAgentConfig(agent.id);
  //     setAgents(prev => prev.filter(a => a.id !== agent.id));
  //   } catch (error) {
  //     console.error('Erro ao deletar agente:', error);
  //     alert('Erro ao deletar agente');
  //   }
  // };

  const handleTest = async (agent: Agent) => {
    console.log('handleTest chamado para agente:', agent.name);
    setTestChatAgent({
      name: agent.name,
      type: agent.agent_type,
      displayName: agent.display_name
    });
  };

  const handleDiagnostic = (agent: Agent) => {
    console.log('handleDiagnostic chamado para agente:', agent.name);
    if (agent.agent_type === 'http' || agent.agent_type === 'mcp') {
      setDiagnosticAgent(agent);
      setShowDiagnostic(true);
    }
  };

  const getTypeIcon = (type: string) => {
    switch (type) {
      case 'http': return '🌐';
      case 'mcp': return '🔌';
      case 'file': return '📁';
      default: return '⚙️';
    }
  };

  const filteredAgents = agents
    .filter(agent => filterType === 'all' || agent.agent_type === filterType)
    .filter(agent =>
      (agent.display_name || agent.name).toLowerCase().includes(searchTerm.toLowerCase()) ||
      (agent.description || '').toLowerCase().includes(searchTerm.toLowerCase())
    );

  const columns: DataGridColumn<Agent>[] = [
    { 
      key: 'agent_type', 
      label: 'Tipo',
      width: '60px',
      format: (value) => getTypeIcon(value)
    },
    { 
      key: 'display_name', 
      label: 'Nome',
      truncate: true,
      format: (value, item) => value || item.name
    },
    { 
      key: 'description', 
      label: 'Descrição',
      truncate: true,
      format: (value) => value || 'Sem descrição'
    },
    { 
      key: 'model', 
      label: 'Modelo',
      width: '140px',
      format: (value) => value || 'gpt-4o-mini'
    },
    { 
      key: 'enabled', 
      label: 'Status',
      width: '100px',
      format: (value) => value ? '✅ Ativo' : '⛔ Inativo'
    },
    { 
      key: 'test', 
      label: 'Testar',
      width: '80px',
      action: true,
      actionIcon: '▶️',
      actionLabel: 'Testar agente',
    },
    { 
      key: 'diagnostic', 
      label: 'Info',
      width: '80px',
      action: true,
      actionIcon: '🔍',
      actionLabel: 'Diagnóstico do agente',
    },
    { 
      key: 'edit', 
      label: 'Editar',
      width: '80px',
      action: true,
      actionIcon: '⚙️',
      actionLabel: 'Editar configurações',
    }
  ];

  const toolbarActions: ToolbarAction[] = [
    {
      key: 'new-http',
      label: 'Novo HTTP Agent',
      icon: '🌐',
      onClick: openNewHTTPAgent,
      variant: 'primary',
    },
    {
      key: 'new-mcp',
      label: 'Novo MCP Agent',
      icon: '🔌',
      onClick: openNewMCPAgent,
      variant: 'primary',
    },
    {
      key: 'refresh',
      label: 'Atualizar',
      icon: '🔄',
      onClick: loadAgents,
      variant: 'secondary',
    },
  ];

  if (loading) {
    return (
      <div className="agents-page">
        <Toolbar left={<h1 className="page-toolbar__title">Gerenciar Agentes</h1>} />
        <div className="page-content">
          <div className="loading-message">Carregando agentes...</div>
        </div>
      </div>
    );
  }

  return (
    <div className="agents-page">
      <Toolbar 
        left={<h1 className="page-toolbar__title">Gerenciador de Agentes</h1>}
        actions={toolbarActions}
        searchValue={searchTerm}
        onSearchChange={setSearchTerm}
        searchPlaceholder="Buscar agentes..."
        center={
          <select 
            value={filterType} 
            onChange={(e) => setFilterType(e.target.value)}
            className="type-filter"
          >
            <option value="all">Todos os tipos</option>
            <option value="internal">⚙️ Internos</option>
            <option value="http">🌐 HTTP</option>
            <option value="mcp">🔌 MCP</option>
            <option value="file">📁 File</option>
          </select>
        }
      />

      <div className="page-content">
        <DataGrid
          items={filteredAgents}
          columns={columns}
          label="Lista de agentes"
          getItemId={(agent) => agent.id || agent.name}
          onActivate={(agent) => {
            console.log('onActivate chamado para agente:', agent.name);
            openEditForm(agent);
          }}
          onCellAction={(agent, column) => {
            console.log('onCellAction chamado - coluna:', column.key, 'agente:', agent.name);
            if (column.key === 'test') {
              handleTest(agent);
            } else if (column.key === 'diagnostic') {
              handleDiagnostic(agent);
            } else if (column.key === 'edit') {
              openEditForm(agent);
            }
          }}
        />
      </div>

      {showModal && (
        <SimpleModal
          isOpen={showModal}
          onClose={() => setShowModal(false)}
          title={`Editar ${editingAgent?.display_name || editingAgent?.name}`}
        >
          <div className="modal-form">
            {formError && (
              <div className="form-error">{formError}</div>
            )}

            <div className="form-group">
              <label htmlFor="displayName">Nome de exibição *</label>
              <Input
                id="displayName"
                value={formDisplayName}
                onChange={(e) => setFormDisplayName(e.target.value)}
                placeholder="Nome amigável do agente"
                autoFocus
              />
            </div>

            <div className="form-group">
              <label htmlFor="description">Descrição</label>
              <Textarea
                id="description"
                value={formDescription}
                onChange={(e) => setFormDescription(e.target.value)}
                placeholder="Descrição do agente..."
                rows={3}
              />
            </div>

            <div className="form-group">
              <label htmlFor="model">Modelo *</label>
              <Select
                id="model"
                value={formModel}
                onChange={(e) => setFormModel(e.target.value)}
                options={AVAILABLE_MODELS.map(model => ({ value: model, label: model }))}
              />
            </div>

            <div className="form-group">
              <label htmlFor="systemPrompt">System Prompt</label>
              <Textarea
                id="systemPrompt"
                value={formSystemPrompt}
                onChange={(e) => setFormSystemPrompt(e.target.value)}
                placeholder="Instruções do sistema para este agente..."
                rows={5}
              />
            </div>

            <div className="form-group">
              <Checkbox
                id="enabled"
                checked={formEnabled}
                onChange={(e) => setFormEnabled(e.target.checked)}
                label="Agente ativo"
              />
            </div>

            <div className="modal-actions">
              <Button variant="secondary" onClick={() => setShowModal(false)}>
                Cancelar
              </Button>
              <Button variant="primary" onClick={handleSave} disabled={saving}>
                {saving ? 'Salvando...' : 'Salvar'}
              </Button>
            </div>
          </div>
        </SimpleModal>
      )}
      
      {/* HTTP Agent Editor Modal */}
      {showHTTPEditor && (
        <Modal
          id="http-agent-editor"
          onClose={() => {
            setShowHTTPEditor(false);
            setEditingHTTPAgentId(null);
          }}
          size="xl"
          title=""
        >
          <HTTPAgentEditor
            agentConfigId={editingHTTPAgentId}
            onClose={() => {
              setShowHTTPEditor(false);
              setEditingHTTPAgentId(null);
            }}
            onSaved={() => {
              loadAgents();
            }}
            onDeleted={() => {
              loadAgents();
            }}
          />
        </Modal>
      )}
      
      {/* MCP Agent Editor Modal */}
      {showMCPEditor && (
        <Modal
          id="mcp-agent-editor"
          onClose={() => {
            setShowMCPEditor(false);
            setEditingMCPAgentId(null);
          }}
          size="xl"
          title=""
        >
          <MCPAgentEditor
            mcpAgentId={editingMCPAgentId}
            onClose={() => {
              setShowMCPEditor(false);
              setEditingMCPAgentId(null);
            }}
            onSaved={() => {
              loadAgents();
            }}
            onDeleted={() => {
              loadAgents();
            }}
          />
        </Modal>
      )}

      {/* Agent Test Chat Modal */}
      {testChatAgent && (
        <AgentTestChat
          agentName={testChatAgent.name}
          agentType={testChatAgent.type}
          displayName={testChatAgent.displayName}
          onClose={() => {
            setTestChatAgent(null);
          }}
        />
      )}

      {/* Agent Diagnostic Modal */}
      {showDiagnostic && diagnosticAgent && (
        <AgentDiagnostic
          agentId={diagnosticAgent.id}
          agentName={diagnosticAgent.display_name || diagnosticAgent.name}
          onClose={() => {
            setShowDiagnostic(false);
            setDiagnosticAgent(null);
          }}
        />
      )}
    </div>
  );
}
