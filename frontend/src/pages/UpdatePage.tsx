import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { CheckCircleOutlined, CloseCircleOutlined } from '@ant-design/icons';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { StartUpdate } from '@wailsjs/go/main/App';
import { useUIStore } from '../store/uiStore';
import { useContentPageLandmarks } from '../hooks/useContentPageLandmarks';
import './UpdatePage.css';

interface ProgressEvent {
  phase: string;
  bytesDownloaded: number;
  totalBytes: number;
  percentage: number;
}

interface UpdateCompletedEvent {
  message?: string;
}

interface UpdateErrorEvent {
  error?: string;
}

type UpdatePhase = 'idle' | 'downloading' | 'verifying' | 'installing' | 'completed' | 'error';

export default function UpdatePage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { addToast } = useUIStore();
  useContentPageLandmarks({ pageClass: 'update-page' });

  const getErrorMessage = (error: unknown) =>
    error instanceof Error ? error.message : String(error ?? '');
  
  const [phase, setPhase] = useState<UpdatePhase>('idle');
  const [progress, setProgress] = useState(0);
  const [bytesDownloaded, setBytesDownloaded] = useState(0);
  const [totalBytes, setTotalBytes] = useState(0);
  const [errorMessage, setErrorMessage] = useState('');

  useEffect(() => {
    // Escuta eventos de progresso
    const unsubProgress = EventsOn('update:progress', (data: ProgressEvent) => {
      setPhase(data.phase as UpdatePhase);
      setProgress(data.percentage);
      setBytesDownloaded(data.bytesDownloaded);
      setTotalBytes(data.totalBytes);
    });

    // Escuta evento de início
    const unsubStarted = EventsOn('update:started', () => {
      setPhase('downloading');
      setProgress(0);
    });

    // Escuta evento de conclusão
    const unsubCompleted = EventsOn('update:completed', (data: UpdateCompletedEvent) => {
      setPhase('completed');
      setProgress(100);
      addToast(data.message || t('update.phases.completed'), 'success');
    });

    // Escuta evento de erro
    const unsubError = EventsOn('update:error', (data: UpdateErrorEvent) => {
      const message = data.error || t('update.errors.unknown');
      setPhase('error');
      setErrorMessage(message);
      addToast(t('update.errors.failed', { error: message }), 'error');
    });

    // NÃO chama ApplyUpdate aqui - StartUpdate() já fez isso!
    // O AboutPage chama StartUpdate() que navega para cá e inicia o processo
    return () => {
      unsubProgress();
      unsubStarted();
      unsubCompleted();
      unsubError();
    };
  }, [addToast, t]); // Removido autoStarted da dependência

  const handleRetryUpdate = async () => {
    setPhase('idle');
    setErrorMessage('');
    try {
      await StartUpdate();
      // StartUpdate() navega para esta página e inicia o processo
    } catch (error: unknown) {
      const message = getErrorMessage(error) || t('update.errors.start');
      setPhase('error');
      setErrorMessage(message);
      addToast(message, 'error');
    }
  };

  const getPhaseText = () => {
    switch (phase) {
      case 'idle':
        return t('update.phases.preparing');
      case 'downloading':
        return t('update.phases.downloading');
      case 'verifying':
        return t('update.phases.verifying');
      case 'installing':
        return t('update.phases.installing');
      case 'completed':
        return t('update.phases.completed');
      case 'error':
        return t('update.phases.error');
      default:
        return t('update.phases.processing');
    }
  };

  const formatBytes = (bytes: number) => {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return Math.round((bytes / Math.pow(k, i)) * 100) / 100 + ' ' + sizes[i];
  };

  return (
    <div className="update-page">
      <div className="update-container">
        <div className="update-header">
          <h1>{t('update.pageTitle')}</h1>
        </div>

        <div className="update-content">
          {phase !== 'error' && phase !== 'completed' && (
            <>
              <div className="update-status">
                <h2>{getPhaseText()}</h2>
                {phase === 'downloading' && totalBytes > 0 && (
                  <p className="download-info">
                    {formatBytes(bytesDownloaded)} / {formatBytes(totalBytes)}
                  </p>
                )}
              </div>

              <div className="progress-container">
                <div className="progress-bar">
                  <div 
                    className="progress-fill"
                    style={{ width: `${progress}%` }}
                  />
                </div>
                <div className="progress-text">{progress.toFixed(1)}%</div>
              </div>

              <div className="update-spinner">
                <div className="spinner" />
              </div>

              <p className="update-message">
                {t('update.message.wait')}
              </p>
            </>
          )}

          {phase === 'completed' && (
            <div className="update-success">
              <div className="success-icon">
                <CheckCircleOutlined aria-hidden="true" />
              </div>
              <h2>{t('update.successTitle')}</h2>
              <p>{t('update.successDesc')}</p>
              <div className="update-actions">
                <button 
                  className="btn-primary"
                  onClick={() => window.location.reload()}
                >
                  {t('update.buttons.restart')}
                </button>
                <button 
                  className="btn-secondary"
                  onClick={() => navigate('/')}
                >
                  {t('update.buttons.backToChat')}
                </button>
              </div>
            </div>
          )}

          {phase === 'error' && (
            <div className="update-error">
              <div className="error-icon">
                <CloseCircleOutlined aria-hidden="true" />
              </div>
              <h2>{t('update.errorTitle')}</h2>
              <p className="error-message">{errorMessage}</p>
              <div className="update-actions">
                <button 
                  className="btn-primary"
                  onClick={handleRetryUpdate}
                >
                  {t('update.buttons.retry')}
                </button>
                <button 
                  className="btn-secondary"
                  onClick={() => navigate('/')}
                >
                  {t('update.buttons.backToChat')}
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
