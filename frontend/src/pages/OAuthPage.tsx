import { useState, useEffect } from 'react';
// import { useTranslation } from 'react-i18next';
import { 
  GetOAuthProviders,
  GetOAuthConnections, 
  StartOAuthFlow, 
  DisconnectOAuth,
  RefreshOAuthConnection 
} from '../../wailsjs/go/main/App';
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime';
import { Toolbar } from '../components/ui/Toolbar';
import './OAuthPage.css';

interface OAuthProvider {
  id: string;
  name: string;
  icon: string;
  is_configured: boolean;
}

interface OAuthConnection {
  id: number;
  provider_id: string;
  user_email?: string;
  user_name?: string;
  is_expired: boolean;
  expires_at?: string;
  provider_name?: string;
  provider_icon?: string;
  scopes?: string;
  last_used_at?: string;
  created_at?: string;
}

export default function OAuthPage() {
  // const { t } = useTranslation();
  const [providers, setProviders] = useState<OAuthProvider[]>([]);
  const [connections, setConnections] = useState<OAuthConnection[]>([]);
  const [loading, setLoading] = useState(true);
  const [authorizing, setAuthorizing] = useState(false);
  const [_authorizingProvider, setAuthorizingProvider] = useState<string | null>(null);
  const [error, setError] = useState('');

  useEffect(() => {
    loadData();
  }, []);

  const loadData = async () => {
    setLoading(true);
    setError('');
    
    try {
      const [providersData, connectionsData] = await Promise.all([
        GetOAuthProviders(),
        GetOAuthConnections()
      ]);
      
      setProviders(providersData || []);
      setConnections(connectionsData || []);
    } catch (err: any) {
      setError('Erro ao carregar: ' + (err.message || err));
    } finally {
      setLoading(false);
    }
  };

  const handleConnect = async (provider: OAuthProvider) => {
    if (!provider.is_configured) {
      alert('Este provider ainda não está configurado no backend.');
      return;
    }

    setAuthorizing(true);
    setAuthorizingProvider(provider.id);
    setError('');

    try {
      // Define scopes padrão baseado no provider
      const scopes = getDefaultScopes(provider.id);
      
      const authURL = await StartOAuthFlow(provider.id, scopes);
      
      // Abre o navegador
      BrowserOpenURL(authURL);
      
      // Recarrega após um tempo para verificar se conectou
      setTimeout(async () => {
        await loadData();
        setAuthorizing(false);
        setAuthorizingProvider(null);
      }, 3000);
      
    } catch (err: any) {
      setError('Erro na autorização: ' + (err.message || err));
      setAuthorizing(false);
      setAuthorizingProvider(null);
    }
  };

  const handleDisconnect = async (connectionId: number) => {
    if (!confirm('Deseja desconectar esta conta?')) return;
    
    try {
      await DisconnectOAuth(connectionId);
      await loadData();
    } catch (err: any) {
      setError('Erro ao desconectar: ' + (err.message || err));
    }
  };

  const handleRefresh = async (connectionId: number) => {
    try {
      await RefreshOAuthConnection(connectionId);
      await loadData();
    } catch (err: any) {
      setError('Erro ao renovar token: ' + (err.message || err));
    }
  };

  const getDefaultScopes = (providerId: string): string[] => {
    const scopesMap: Record<string, string[]> = {
      google: [
        'https://www.googleapis.com/auth/gmail.readonly',
        'https://www.googleapis.com/auth/calendar',
        'https://www.googleapis.com/auth/drive.file'
      ],
      microsoft: [
        'Mail.Read',
        'Calendars.ReadWrite',
        'Files.ReadWrite.All'
      ],
      github: ['repo', 'gist'],
      slack: ['channels:read', 'chat:write'],
    };

    return scopesMap[providerId] || [];
  };

  const getConnectionsForProvider = (providerId: string) => {
    return connections.filter(c => c.provider_id === providerId);
  };

  if (loading) {
    return (
      <div className="oauth-page">
        <Toolbar left={<h1 className="page-toolbar__title">Conexões OAuth</h1>} />
        <div className="page-content">
          <div className="loading-message">Carregando conexões...</div>
        </div>
      </div>
    );
  }

  return (
    <div className="oauth-page">
      <Toolbar left={<h1 className="page-toolbar__title">Conexões OAuth</h1>} />

      <div className="page-content">
        <div className="oauth-description">
          <p>Conecte suas contas para automações avançadas e integração com serviços externos.</p>
        </div>

        {error && (
          <div className="error-message">{error}</div>
        )}

        {authorizing && (
          <div className="authorizing-overlay">
            <div className="authorizing-card">
              <div className="authorizing-spinner">⏳</div>
              <h3>Autorizando...</h3>
              <p>Complete a autorização no navegador.</p>
              <p className="hint">Após autorizar, volte para esta janela.</p>
            </div>
          </div>
        )}

        <div className="providers-grid">
          {providers.map(provider => {
            const providerConnections = getConnectionsForProvider(provider.id);
            const hasConnection = providerConnections.length > 0;

            return (
              <div 
                key={provider.id}
                className={`provider-card ${!provider.is_configured ? 'disabled' : ''}`}
              >
                <div className="provider-header">
                  <span className="provider-icon">{provider.icon}</span>
                  <span className="provider-name">{provider.name}</span>
                  {!provider.is_configured && (
                    <span className="badge warning">Não configurado</span>
                  )}
                </div>

                {/* Conexões existentes */}
                {providerConnections.map(conn => (
                  <div 
                    key={conn.id}
                    className={`connection-item ${conn.is_expired ? 'expired' : ''}`}
                  >
                    <div className="connection-info">
                      <span className="user-email">
                        {conn.user_email || conn.user_name || 'Conta conectada'}
                      </span>
                      {conn.is_expired ? (
                        <span className="badge error">Expirado</span>
                      ) : (
                        <span className="badge success">Ativo</span>
                      )}
                    </div>
                    <div className="connection-actions">
                      {conn.is_expired && (
                        <button 
                          onClick={() => handleRefresh(conn.id)}
                          className="btn-icon"
                          title="Renovar token"
                        >
                          🔄
                        </button>
                      )}
                      <button 
                        onClick={() => handleDisconnect(conn.id)}
                        className="btn-icon danger"
                        title="Desconectar"
                        aria-label="Desconectar conta"
                      >
                        🗑️
                      </button>
                    </div>
                  </div>
                ))}

                {/* Botão de conectar */}
                <div className="provider-actions">
                  {provider.is_configured && (
                    <button
                      onClick={() => handleConnect(provider)}
                      className="btn-connect"
                      disabled={authorizing}
                    >
                      {hasConnection ? '➕ Adicionar outra conta' : '🔗 Conectar'}
                    </button>
                  )}
                </div>
              </div>
            );
          })}
        </div>

        {providers.length === 0 && (
          <div className="empty-state">
            <p>Nenhum provider OAuth configurado.</p>
          </div>
        )}
      </div>
    </div>
  );
}
