import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { GetAppVersion, CheckForUpdates, StartUpdate } from '../../wailsjs/go/main/App';
import { useUIStore } from '../store/uiStore';
import './AboutPage.css';

interface UpdateInfo {
  available: boolean;
  currentVersion: string;
  latestVersion: string;
  releaseNotes?: string;
  releaseDate?: string;
  downloadSize?: number;
}

export default function AboutPage() {
  const navigate = useNavigate();
  const { addToast } = useUIStore();
  
  const [version, setVersion] = useState('');
  const [loading, setLoading] = useState(false);
  const [checking, setChecking] = useState(false);
  const [updateInfo, setUpdateInfo] = useState<UpdateInfo | null>(null);

  useEffect(() => {
    loadVersion();
  }, []);

  const loadVersion = async () => {
    try {
      const currentVersion = await GetAppVersion();
      setVersion(currentVersion);
    } catch (error) {
      console.error('Erro ao obter versão:', error);
      setVersion('Desconhecida');
    }
  };

  const handleCheckForUpdates = async () => {
    setChecking(true);
    try {
      const info = await CheckForUpdates();
      setUpdateInfo(info);
      
      if (info.available) {
        addToast(`Nova versão disponível: ${info.latestVersion}`, 'success');
      } else {
        addToast('Você já está usando a versão mais recente!', 'success');
      }
    } catch (error: any) {
      console.error('Erro ao verificar atualizações:', error);
      addToast(error.message || 'Erro ao verificar atualizações', 'error');
    } finally {
      setChecking(false);
    }
  };

  const handleStartUpdate = async () => {
    setLoading(true);
    try {
      await StartUpdate();
      // O backend irá emitir evento navigate:update que será capturado
      // pela App.tsx e navegará para /update
    } catch (error: any) {
      console.error('Erro ao iniciar atualização:', error);
      addToast(error.message || 'Erro ao iniciar atualização', 'error');
      setLoading(false);
    }
  };

  const formatDate = (dateStr?: string) => {
    if (!dateStr) return 'Desconhecida';
    try {
      const date = new Date(dateStr);
      return date.toLocaleDateString('pt-BR', {
        year: 'numeric',
        month: 'long',
        day: 'numeric',
      });
    } catch {
      return dateStr;
    }
  };

  const formatBytes = (bytes?: number) => {
    if (!bytes) return '';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return Math.round((bytes / Math.pow(k, i)) * 100) / 100 + ' ' + sizes[i];
  };

  return (
    <div className="about-page">
      <div className="about-container">
        <div className="about-header">
          <div className="app-icon">🤖</div>
          <h1>Assistente</h1>
          <p className="app-tagline">Seu assistente de IA inteligente</p>
        </div>

        <div className="about-content">
          <div className="info-section">
            <h2>Informações do Aplicativo</h2>
            <div className="info-grid">
              <div className="info-item">
                <span className="info-label">Versão Instalada:</span>
                <span className="info-value">{version || 'Carregando...'}</span>
              </div>
              <div className="info-item">
                <span className="info-label">Desenvolvedor:</span>
                <span className="info-value">Inclunet</span>
              </div>
              <div className="info-item">
                <span className="info-label">Licença:</span>
                <span className="info-value">Proprietária</span>
              </div>
            </div>
          </div>

          <div className="update-section">
            <h2>Atualizações</h2>
            
            {!updateInfo && (
              <div className="update-check">
                <p>Verifique se há uma nova versão disponível do aplicativo.</p>
                <button 
                  className="btn-check-updates"
                  onClick={handleCheckForUpdates}
                  disabled={checking}
                >
                  {checking ? (
                    <>
                      <span className="btn-spinner" />
                      Verificando...
                    </>
                  ) : (
                    <>
                      🔍 Verificar Atualizações
                    </>
                  )}
                </button>
              </div>
            )}

            {updateInfo && !updateInfo.available && (
              <div className="update-current">
                <div className="status-icon">✅</div>
                <h3>Você está atualizado!</h3>
                <p>Versão {updateInfo.currentVersion} é a mais recente.</p>
                <button 
                  className="btn-check-again"
                  onClick={handleCheckForUpdates}
                  disabled={checking}
                >
                  Verificar Novamente
                </button>
              </div>
            )}

            {updateInfo && updateInfo.available && (
              <div className="update-available">
                <div className="status-icon">🆕</div>
                <h3>Nova Versão Disponível!</h3>
                
                <div className="version-info">
                  <div className="version-row">
                    <span>Versão Atual:</span>
                    <strong>{updateInfo.currentVersion}</strong>
                  </div>
                  <div className="version-row">
                    <span>Nova Versão:</span>
                    <strong className="new-version">{updateInfo.latestVersion}</strong>
                  </div>
                  {updateInfo.releaseDate && (
                    <div className="version-row">
                      <span>Data de Lançamento:</span>
                      <strong>{formatDate(updateInfo.releaseDate)}</strong>
                    </div>
                  )}
                  {updateInfo.downloadSize && (
                    <div className="version-row">
                      <span>Tamanho:</span>
                      <strong>{formatBytes(updateInfo.downloadSize)}</strong>
                    </div>
                  )}
                </div>

                {updateInfo.releaseNotes && (
                  <div className="release-notes">
                    <h4>Novidades desta versão:</h4>
                    <div className="notes-content">
                      {updateInfo.releaseNotes}
                    </div>
                  </div>
                )}

                <div className="update-actions">
                  <button 
                    className="btn-update-now"
                    onClick={handleStartUpdate}
                    disabled={loading}
                  >
                    {loading ? (
                      <>
                        <span className="btn-spinner" />
                        Iniciando...
                      </>
                    ) : (
                      <>
                        ⬇️ Atualizar Agora
                      </>
                    )}
                  </button>
                  <button 
                    className="btn-update-later"
                    onClick={() => setUpdateInfo(null)}
                  >
                    Mais Tarde
                  </button>
                </div>
              </div>
            )}
          </div>

          <div className="links-section">
            <h2>Links Úteis</h2>
            <div className="links-grid">
              <a 
                href="https://github.com/inclunet/assistente" 
                target="_blank" 
                rel="noopener noreferrer"
                className="link-item"
              >
                <span className="link-icon">🔗</span>
                <span>Repositório GitHub</span>
              </a>
              <a 
                href="https://github.com/inclunet/assistente/issues" 
                target="_blank" 
                rel="noopener noreferrer"
                className="link-item"
              >
                <span className="link-icon">🐛</span>
                <span>Reportar Bug</span>
              </a>
              <a 
                href="https://github.com/inclunet/assistente/releases" 
                target="_blank" 
                rel="noopener noreferrer"
                className="link-item"
              >
                <span className="link-icon">📋</span>
                <span>Notas de Versão</span>
              </a>
            </div>
          </div>
        </div>

        <div className="about-footer">
          <button 
            className="btn-back"
            onClick={() => navigate('/')}
          >
            ← Voltar
          </button>
        </div>
      </div>
    </div>
  );
}
