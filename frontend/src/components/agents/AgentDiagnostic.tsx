import { useState, useEffect } from 'react';
import { GetHTTPAgentFull } from '../../../wailsjs/go/main/App';
import { Modal } from '../ui/Modal';
import { Button } from '../ui/Button';
import './AgentDiagnostic.css';

interface AgentDiagnosticProps {
  agentId: number;
  agentName: string;
  onClose: () => void;
}

export function AgentDiagnostic({ agentId, agentName, onClose }: AgentDiagnosticProps) {
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<any>(null);
  const [error, setError] = useState('');

  useEffect(() => {
    loadAgentData();
  }, [agentId]);

  const loadAgentData = async () => {
    setLoading(true);
    setError('');
    
    try {
      const agent = await GetHTTPAgentFull(agentId);
      setData(agent);
      console.log('[AgentDiagnostic] Dados do agente:', agent);
    } catch (err: any) {
      setError('Erro ao carregar dados: ' + (err.message || err));
    } finally {
      setLoading(false);
    }
  };

  if (loading) {
    return (
      <Modal id="agent-diagnostic" onClose={onClose} title={`Diagnóstico: ${agentName}`}>
        <div className="agent-diagnostic">
          <p>Carregando dados...</p>
        </div>
      </Modal>
    );
  }

  if (error) {
    return (
      <Modal id="agent-diagnostic" onClose={onClose} title={`Diagnóstico: ${agentName}`}>
        <div className="agent-diagnostic">
          <div className="diagnostic-error" role="alert">
            {error}
          </div>
        </div>
      </Modal>
    );
  }

  const endpoints = data?.endpoints || [];
  const hasEndpoints = endpoints.length > 0;

  return (
    <Modal 
      id="agent-diagnostic" 
      onClose={onClose} 
      title={`Diagnóstico: ${agentName}`}
      size="lg"
    >
      <div className="agent-diagnostic" role="region" aria-label="Informações de diagnóstico do agente">
        <section className="diagnostic-section">
          <h3>Informações Básicas</h3>
          <dl className="diagnostic-list">
            <dt>Nome:</dt>
            <dd>{data?.name || 'N/A'}</dd>
            
            <dt>Nome de Exibição:</dt>
            <dd>{data?.display_name || 'N/A'}</dd>
            
            <dt>Modelo:</dt>
            <dd>{data?.model || 'N/A'}</dd>
            
            <dt>Habilitado:</dt>
            <dd>{data?.enabled ? 'Sim' : 'Não'}</dd>
            
            <dt>Base URL:</dt>
            <dd>{data?.base_url || 'N/A'}</dd>
            
            <dt>Tipo de Autenticação:</dt>
            <dd>{data?.auth_type || 'none'}</dd>
          </dl>
        </section>

        <section className="diagnostic-section">
          <h3>System Prompt</h3>
          <pre className="diagnostic-code" role="region" aria-label="System prompt do agente">
            {data?.system_prompt || 'Nenhum system prompt configurado'}
          </pre>
        </section>

        <section className="diagnostic-section">
          <h3>Endpoints ({endpoints.length})</h3>
          
          {!hasEndpoints && (
            <p className="diagnostic-warning" role="alert">
              ⚠️ Nenhum endpoint configurado! O agente não terá tools para usar.
            </p>
          )}

          {hasEndpoints && endpoints.map((endpoint: any, idx: number) => (
            <div key={idx} className="diagnostic-endpoint">
              <h4>Endpoint {idx + 1}: {endpoint.name}</h4>
              
              <dl className="diagnostic-list">
                <dt>Descrição:</dt>
                <dd>{endpoint.description || 'Sem descrição'}</dd>
                
                <dt>Método:</dt>
                <dd>{endpoint.method}</dd>
                
                <dt>Path Template:</dt>
                <dd><code>{endpoint.path_template}</code></dd>
                
                <dt>Parameters (JSON Schema):</dt>
                <dd>
                  {endpoint.parameters ? (
                    <pre className="diagnostic-code" role="region" aria-label={`Parameters do endpoint ${endpoint.name}`}>
                      {JSON.stringify(JSON.parse(endpoint.parameters), null, 2)}
                    </pre>
                  ) : (
                    <p className="diagnostic-error" role="alert">
                      ❌ <strong>PROBLEMA ENCONTRADO:</strong> Campo parameters está vazio! 
                      O LLM não conseguirá chamar esta tool sem o schema de parâmetros.
                    </p>
                  )}
                </dd>
                
                {endpoint.query_template && (
                  <>
                    <dt>Query Template:</dt>
                    <dd><code>{endpoint.query_template}</code></dd>
                  </>
                )}
                
                {endpoint.body_template && (
                  <>
                    <dt>Body Template:</dt>
                    <dd>
                      <pre className="diagnostic-code" role="region" aria-label="Body template">
                        {endpoint.body_template}
                      </pre>
                    </dd>
                  </>
                )}
              </dl>
            </div>
          ))}
        </section>

        <section className="diagnostic-section">
          <h3>Diagnóstico</h3>
          <div className="diagnostic-checks">
            <div className={`diagnostic-check ${data?.enabled ? 'check-ok' : 'check-error'}`}>
              {data?.enabled ? '✅' : '❌'} Agente está {data?.enabled ? 'habilitado' : 'desabilitado'}
            </div>
            
            <div className={`diagnostic-check ${hasEndpoints ? 'check-ok' : 'check-error'}`}>
              {hasEndpoints ? '✅' : '❌'} {hasEndpoints ? `${endpoints.length} endpoint(s) configurado(s)` : 'Nenhum endpoint configurado'}
            </div>
            
            {endpoints.map((endpoint: any, idx: number) => (
              <div key={idx} className={`diagnostic-check ${endpoint.parameters ? 'check-ok' : 'check-error'}`}>
                {endpoint.parameters ? '✅' : '❌'} Endpoint "{endpoint.name}" {endpoint.parameters ? 'tem' : 'NÃO TEM'} parameters definidos
              </div>
            ))}
            
            <div className={`diagnostic-check ${data?.model ? 'check-ok' : 'check-warning'}`}>
              {data?.model ? '✅' : '⚠️'} Modelo: {data?.model || 'Não especificado (usará default)'}
            </div>
          </div>
        </section>

        <section className="diagnostic-section">
          <h3>Recomendações</h3>
          <ul className="diagnostic-recommendations">
            {!hasEndpoints && (
              <li className="recommendation-error">
                <strong>CRÍTICO:</strong> Crie pelo menos um endpoint para o agente poder executar ações.
              </li>
            )}
            
            {endpoints.some((e: any) => !e.parameters) && (
              <li className="recommendation-error">
                <strong>CRÍTICO:</strong> Alguns endpoints não têm o campo "parameters" definido. 
                Adicione um JSON Schema para cada endpoint. Exemplo:
                <pre className="diagnostic-code">
{`{
  "type": "object",
  "properties": {
    "cep": {
      "type": "string",
      "description": "CEP no formato 00000-000"
    }
  },
  "required": ["cep"]
}`}
                </pre>
              </li>
            )}
            
            {!data?.system_prompt && (
              <li className="recommendation-warning">
                <strong>Recomendado:</strong> Adicione um system prompt personalizado para melhor contexto.
              </li>
            )}
          </ul>
        </section>

        <div className="diagnostic-actions">
          <Button onClick={onClose} variant="primary">
            Fechar
          </Button>
        </div>
      </div>
    </Modal>
  );
}
