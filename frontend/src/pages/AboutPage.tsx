import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { GetAppVersion, CheckForUpdates, StartUpdate } from '@wailsjs/go/main/App';
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
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { addToast } = useUIStore();
  const getErrorMessage = (error: unknown) =>
    error instanceof Error ? error.message : String(error ?? '');
  
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
      setVersion(t('about.versionUnknown'));
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
        addToast(t('about.upToDateDesc', { version: info.currentVersion }), 'success');
      }
    } catch (error: unknown) {
      console.error('Erro ao verificar atualizações:', error);
      addToast(getErrorMessage(error) || 'Erro ao verificar atualizações', 'error');
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
    } catch (error: unknown) {
      console.error('Erro ao iniciar atualização:', error);
      addToast(getErrorMessage(error) || 'Erro ao iniciar atualização', 'error');
      setLoading(false);
    }
  };

  const formatDate = (dateStr?: string) => {
    if (!dateStr) return t('about.versionUnknown');
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
          <h1>{t('about.pageTitle')}</h1>
          <p className="app-tagline">{t('about.tagline')}</p>
        </div>

        <div className="about-content">
          <div className="info-section">
            <h2>{t('about.sections.appInfo')}</h2>
            <div className="info-grid">
              <div className="info-item">
                <span className="info-label">{t('about.labels.installedVersion')}</span>
                <span className="info-value">{version || t('about.loading')}</span>
              </div>
              <div className="info-item">
                <span className="info-label">{t('about.labels.developer')}</span>
                <span className="info-value">Inclunet</span>
              </div>
              <div className="info-item">
                <span className="info-label">{t('about.labels.license')}</span>
                <span className="info-value">Proprietária</span>
              </div>
            </div>
          </div>

          <div className="update-section">
            <h2>{t('about.sections.updates')}</h2>
            
            {!updateInfo && (
              <div className="update-check">
                <p>{t('about.checkUpdatesDesc')}</p>
                <button 
                  className="btn-check-updates"
                  onClick={handleCheckForUpdates}
                  disabled={checking}
                >
                  {checking ? (
                    <>
                      <span className="btn-spinner" />
                      {t('about.buttons.checking')}
                    </>
                  ) : (
                    <>
                      {t('about.buttons.checkUpdates')}
                    </>
                  )}
                </button>
              </div>
            )}

            {updateInfo && !updateInfo.available && (
              <div className="update-current">
                <div className="status-icon">✅</div>
                <h3>{t('about.upToDateTitle')}</h3>
                <p>{t('about.upToDateDesc', { version: updateInfo.currentVersion })}</p>
                <button 
                  className="btn-check-again"
                  onClick={handleCheckForUpdates}
                  disabled={checking}
                >
                  {t('about.buttons.checkAgain')}
                </button>
              </div>
            )}

            {updateInfo && updateInfo.available && (
              <div className="update-available">
                <div className="status-icon">🆕</div>
                <h3>{t('about.newVersionTitle')}</h3>
                
                <div className="version-info">
                  <div className="version-row">
                    <span>{t('about.labels.currentVersion')}</span>
                    <strong>{updateInfo.currentVersion}</strong>
                  </div>
                  <div className="version-row">
                    <span>{t('about.labels.newVersion')}</span>
                    <strong className="new-version">{updateInfo.latestVersion}</strong>
                  </div>
                  {updateInfo.releaseDate && (
                    <div className="version-row">
                      <span>{t('about.labels.releaseDate')}</span>
                      <strong>{formatDate(updateInfo.releaseDate)}</strong>
                    </div>
                  )}
                  {updateInfo.downloadSize && (
                    <div className="version-row">
                      <span>{t('about.labels.size')}</span>
                      <strong>{formatBytes(updateInfo.downloadSize)}</strong>
                    </div>
                  )}
                </div>

                {updateInfo.releaseNotes && (
                  <div className="release-notes">
                    <h4>{t('about.releaseNotesTitle')}</h4>
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
                        {t('about.buttons.starting')}
                      </>
                    ) : (
                      <>
                        {t('about.buttons.updateNow')}
                      </>
                    )}
                  </button>
                  <button 
                    className="btn-update-later"
                    onClick={() => setUpdateInfo(null)}
                  >
                    {t('about.buttons.later')}
                  </button>
                </div>
              </div>
            )}
          </div>

          <div className="links-section">
            <h2>{t('about.sections.usefulLinks')}</h2>
            <div className="links-grid">
              <a 
                href="https://github.com/inclunet/assistente" 
                target="_blank" 
                rel="noopener noreferrer"
                className="link-item"
              >
                <span className="link-icon">🔗</span>
                <span>{t('about.links.github')}</span>
              </a>
              <a 
                href="https://github.com/inclunet/assistente/issues" 
                target="_blank" 
                rel="noopener noreferrer"
                className="link-item"
              >
                <span className="link-icon">🐛</span>
                <span>{t('about.links.reportBug')}</span>
              </a>
              <a 
                href="https://github.com/inclunet/assistente/releases" 
                target="_blank" 
                rel="noopener noreferrer"
                className="link-item"
              >
                <span className="link-icon">📋</span>
                <span>{t('about.links.releaseNotes')}</span>
              </a>
            </div>
          </div>
        </div>

        <div className="about-footer">
          <button 
            className="btn-back"
            onClick={() => navigate('/')}
          >
            {t('about.buttons.back')}
          </button>
        </div>
      </div>
    </div>
  );
}
