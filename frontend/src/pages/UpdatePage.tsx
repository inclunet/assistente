import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { EventsOn } from '../../wailsjs/runtime/runtime';
import { ApplyUpdate } from '../../wailsjs/go/main/App';
import { useUIStore } from '../store/uiStore';
import './UpdatePage.css';

interface ProgressEvent {
  phase: string;
  bytesDownloaded: number;
  totalBytes: number;
  percentage: number;
}

type UpdatePhase = 'idle' | 'downloading' | 'verifying' | 'installing' | 'completed' | 'error';

export default function UpdatePage() {
  const navigate = useNavigate();
  const { addToast } = useUIStore();
  
  const [phase, setPhase] = useState<UpdatePhase>('idle');
  const [progress, setProgress] = useState(0);
  const [bytesDownloaded, setBytesDownloaded] = useState(0);
  const [totalBytes, setTotalBytes] = useState(0);
  const [errorMessage, setErrorMessage] = useState('');
  const [autoStarted, setAutoStarted] = useState(false);

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
    const unsubCompleted = EventsOn('update:completed', (data: any) => {
      setPhase('completed');
      setProgress(100);
      addToast(data.message || 'Atualização concluída!', 'success');
    });

    // Escuta evento de erro
    const unsubError = EventsOn('update:error', (data: any) => {
      setPhase('error');
      setErrorMessage(data.error || 'Erro desconhecido');
      addToast('Erro ao atualizar: ' + data.error, 'error');
    });

    // Auto-inicia a atualização se não foi iniciada ainda
    if (!autoStarted) {
      setAutoStarted(true);
      handleStartUpdate();
    }

    return () => {
      unsubProgress();
      unsubStarted();
      unsubCompleted();
      unsubError();
    };
  }, [autoStarted, addToast]);

  const handleStartUpdate = async () => {
    try {
      await ApplyUpdate();
    } catch (error: any) {
      setPhase('error');
      setErrorMessage(error.message || 'Erro ao iniciar atualização');
      addToast('Erro ao iniciar atualização', 'error');
    }
  };

  const getPhaseText = () => {
    switch (phase) {
      case 'idle':
        return 'Preparando atualização...';
      case 'downloading':
        return 'Baixando atualização...';
      case 'verifying':
        return 'Verificando integridade...';
      case 'installing':
        return 'Instalando atualização...';
      case 'completed':
        return 'Atualização concluída!';
      case 'error':
        return 'Erro na atualização';
      default:
        return 'Processando...';
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
          <h1>🔄 Atualização do Sistema</h1>
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
                Por favor, aguarde. Não feche o aplicativo durante a atualização.
              </p>
            </>
          )}

          {phase === 'completed' && (
            <div className="update-success">
              <div className="success-icon">✅</div>
              <h2>Atualização Instalada com Sucesso!</h2>
              <p>Reinicie o aplicativo para aplicar as mudanças.</p>
              <div className="update-actions">
                <button 
                  className="btn-primary"
                  onClick={() => window.location.reload()}
                >
                  Reiniciar Agora
                </button>
                <button 
                  className="btn-secondary"
                  onClick={() => navigate('/')}
                >
                  Voltar ao Chat
                </button>
              </div>
            </div>
          )}

          {phase === 'error' && (
            <div className="update-error">
              <div className="error-icon">❌</div>
              <h2>Erro na Atualização</h2>
              <p className="error-message">{errorMessage}</p>
              <div className="update-actions">
                <button 
                  className="btn-primary"
                  onClick={handleStartUpdate}
                >
                  Tentar Novamente
                </button>
                <button 
                  className="btn-secondary"
                  onClick={() => navigate('/')}
                >
                  Voltar ao Chat
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
