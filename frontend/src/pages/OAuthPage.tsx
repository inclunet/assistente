import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { 
  GetOAuthConnections, 
  StartOAuthFlow, 
  DisconnectOAuth 
} from '../../wailsjs/go/main/App';
import './OAuthPage.css';

interface OAuthService {
  id: string;
  name: string;
  icon: string;
  connected: boolean;
  expires_at?: string;
  account?: string;
}

export default function OAuthPage() {
  const { t } = useTranslation();
  const [services, setServices] = useState<OAuthService[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    loadOAuthStatus();
  }, []);

  const loadOAuthStatus = async () => {
    setLoading(true);
    try {
      const result = await GetOAuthConnections();
      // Adaptar resultado do backend para nossa interface
      const servicesMap = new Map<string, any>();
      result.forEach((conn: any) => {
        servicesMap.set(conn.provider, {
          id: conn.provider,
          name: conn.provider === 'google_docs' ? 'Google Docs' : 'Google Drive',
          icon: conn.provider === 'google_docs' ? '📄' : '💾',
          connected: conn.is_active,
          expires_at: conn.expires_at,
          account: conn.account_email,
        });
      });
      
      // Adicionar serviços padrão se não existirem
      if (!servicesMap.has('google_docs')) {
        servicesMap.set('google_docs', { id: 'google_docs', name: 'Google Docs', icon: '📄', connected: false });
      }
      if (!servicesMap.has('google_drive')) {
        servicesMap.set('google_drive', { id: 'google_drive', name: 'Google Drive', icon: '💾', connected: false });
      }
      
      setServices(Array.from(servicesMap.values()));
    } catch (error) {
      console.error('Erro ao carregar status OAuth:', error);
      // Definir serviços padrão mesmo com erro
      setServices([
        { id: 'google_docs', name: 'Google Docs', icon: '📄', connected: false },
        { id: 'google_drive', name: 'Google Drive', icon: '💾', connected: false },
      ]);
    } finally {
      setLoading(false);
    }
  };

  const handleConnect = async (serviceId: string) => {
    try {
      const scopes = serviceId === 'google_docs' 
        ? ['https://www.googleapis.com/auth/documents']
        : ['https://www.googleapis.com/auth/drive'];
      const authUrl = await StartOAuthFlow(serviceId, scopes);
      // Abrir URL de autorização
      window.open(authUrl, '_blank');
      await loadOAuthStatus();
    } catch (error) {
      console.error('Erro ao conectar OAuth:', error);
      alert(t('oauth.connectError', 'Erro ao conectar serviço'));
    }
  };

  const handleDisconnect = async (serviceId: string) => {
    if (!confirm(t('oauth.confirmDisconnect', 'Desconectar este serviço?'))) return;
    
    try {
      // DisconnectOAuth espera um número (ID da conexão), não string
      // Precisamos buscar o ID da conexão pelo provider
      const connections = await GetOAuthConnections();
      const conn = connections.find((c: any) => c.provider === serviceId);
      if (conn) {
        await DisconnectOAuth(conn.id);
        await loadOAuthStatus();
      }
    } catch (error) {
      console.error('Erro ao desconectar OAuth:', error);
    }
  };

  const formatExpiryDate = (expiresAt?: string) => {
    if (!expiresAt) return '';
    try {
      const date = new Date(expiresAt);
      const now = new Date();
      const diffDays = Math.ceil((date.getTime() - now.getTime()) / (1000 * 60 * 60 * 24));
      
      if (diffDays < 0) return 'Expirado';
      if (diffDays === 0) return 'Expira hoje';
      if (diffDays === 1) return 'Expira amanhã';
      return `Expira em ${diffDays} dias`;
    } catch {
      return '';
    }
  };

  if (loading) {
    return (
      <div className="oauth-page">
        <div className="loading">Carregando serviços OAuth...</div>
      </div>
    );
  }

  return (
    <div className="oauth-page">
      <header className="oauth-header">
        <h1>{t('oauth.title', 'Integrações OAuth')}</h1>
        <p className="subtitle">
          {t('oauth.subtitle', 'Conecte serviços externos para expandir as capacidades do assistente')}
        </p>
      </header>

      <div className="services-grid">
        {services.map((service) => (
          <div key={service.id} className={`service-card ${service.connected ? 'connected' : ''}`}>
            <div className="service-icon">{service.icon}</div>
            <div className="service-info">
              <h3>{service.name}</h3>
              {service.connected ? (
                <div className="connection-details">
                  <p className="status connected">
                    <span className="status-dot"></span>
                    Conectado
                  </p>
                  {service.account && <p className="account">Conta: {service.account}</p>}
                  {service.expires_at && (
                    <p className="expiry">{formatExpiryDate(service.expires_at)}</p>
                  )}
                </div>
              ) : (
                <p className="status disconnected">
                  <span className="status-dot"></span>
                  Não conectado
                </p>
              )}
            </div>
            <div className="service-actions">
              {service.connected ? (
                <button 
                  onClick={() => handleDisconnect(service.id)} 
                  className="btn-disconnect"
                >
                  Desconectar
                </button>
              ) : (
                <button 
                  onClick={() => handleConnect(service.id)} 
                  className="btn-connect"
                >
                  Conectar
                </button>
              )}
            </div>
          </div>
        ))}
      </div>

      {services.length === 0 && (
        <div className="empty-state">
          <p>{t('oauth.empty', 'Nenhum serviço OAuth disponível')}</p>
        </div>
      )}
    </div>
  );
}
