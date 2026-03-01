import React, { useState, useEffect } from 'react';
import { GetConversationTokenStats } from '@wailsjs/go/main/App';
import { EventsOn } from '@wailsjs/runtime/runtime';
import './TokenStatsModal.css';

interface TokenStats {
  conversationId: number;
  promptTokens: number;
  completionTokens: number;
  totalTokens: number;
  messageCount: number;
  mostUsedModel: string;
  contextUsage: number;
  contextLimit: number;
  isNearLimit: boolean;
  isCritical: boolean;
}

interface TokenStatsModalProps {
  conversationId: number;
  isOpen: boolean;
  onClose: () => void;
}

export const TokenStatsModal: React.FC<TokenStatsModalProps> = ({
  conversationId,
  isOpen,
  onClose,
}) => {
  const [stats, setStats] = useState<TokenStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const modalRef = React.useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!isOpen || !conversationId) {
      return;
    }

    // Carrega estatísticas iniciais
    const loadStats = async () => {
      try {
        setLoading(true);
        setError(null);
        const result = await GetConversationTokenStats(conversationId);
        setStats(result);
      } catch (err) {
        console.error('[TokenStatsModal] Erro ao carregar estatísticas:', err);
        setError('Erro ao carregar estatísticas');
      } finally {
        setLoading(false);
      }
    };

    loadStats();

    // Escuta atualizações em tempo real
    const unsubscribe = EventsOn('chat:token_stats', (data: TokenStats) => {
      if (data.conversationId === conversationId) {
        setStats(data);
      }
    });

    return () => unsubscribe();
  }, [conversationId, isOpen]);

  // Gerencia foco para leitores de tela
  useEffect(() => {
    if (isOpen && modalRef.current) {
      // Move foco para o modal quando abre
      modalRef.current.focus();
    }
  }, [isOpen]);

  useEffect(() => {
    if (!isOpen) return;

    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose();
      }
    };

    window.addEventListener('keydown', handleEscape);
    return () => window.removeEventListener('keydown', handleEscape);
  }, [isOpen, onClose]);

  if (!isOpen) return null;

  const formatNumber = (num: number): string => {
    return num.toLocaleString('pt-BR');
  };

  const calculatePercentage = (value: number, total: number): number => {
    if (total === 0) return 0;
    return Math.round((value / total) * 100);
  };

  const getProgressBarColor = (percentage: number): string => {
    if (percentage >= 95) return 'critical';
    if (percentage >= 80) return 'warning';
    return 'normal';
  };

  const estimatedCost = stats ? {
    input: (stats.promptTokens / 1000000) * 0.5, // $0.50 por 1M tokens (exemplo GPT-4)
    output: (stats.completionTokens / 1000000) * 1.5, // $1.50 por 1M tokens
  } : null;

  return (
    <div className="token-stats-modal-overlay" onClick={onClose}>
      <div
        ref={modalRef}
        className="token-stats-modal"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-labelledby="token-stats-title"
        tabIndex={-1}
      >
        <div className="token-stats-modal__header">
          <h2 id="token-stats-title">Estatísticas de Tokens</h2>
          <button
            className="token-stats-modal__close"
            onClick={onClose}
            aria-label="Fechar modal (ESC)"
            title="Fechar (ESC)"
          >
            ✕
          </button>
        </div>

        <div className="token-stats-modal__content">
          {loading && (
            <div className="token-stats-modal__loading">
              <span>Carregando...</span>
            </div>
          )}

          {error && (
            <div className="token-stats-modal__error">
              <span>⚠️ {error}</span>
            </div>
          )}

          {!loading && !error && stats && (
            <>
              {/* Uso do contexto */}
              <section className="token-stats-section">
                <h3>Uso do Contexto</h3>
                <div className="token-stats-context">
                  <div className="token-stats-context__numbers">
                    <span className="token-stats-context__current">
                      {formatNumber(stats.totalTokens)}
                    </span>
                    <span className="token-stats-context__separator">/</span>
                    <span className="token-stats-context__limit">
                      {formatNumber(stats.contextLimit)}
                    </span>
                    <span className="token-stats-context__percentage">
                      ({stats.contextUsage.toFixed(1)}%)
                    </span>
                  </div>
                  <div className="token-stats-progress">
                    <div
                      className={`token-stats-progress__bar token-stats-progress__bar--${getProgressBarColor(stats.contextUsage)}`}
                      style={{ width: `${Math.min(stats.contextUsage, 100)}%` }}
                      role="progressbar"
                      aria-valuenow={stats.contextUsage}
                      aria-valuemin={0}
                      aria-valuemax={100}
                    />
                  </div>
                  {stats.isNearLimit && (
                    <div className="token-stats-warning">
                      {stats.isCritical ? (
                        <span>🔴 Contexto crítico! Considere compactar o histórico.</span>
                      ) : (
                        <span>🟡 Contexto próximo do limite.</span>
                      )}
                    </div>
                  )}
                </div>
              </section>

              {/* Detalhamento de tokens */}
              <section className="token-stats-section">
                <h3>Detalhamento</h3>
                <table className="token-stats-table">
                  <thead>
                    <tr>
                      <th scope="col">Categoria</th>
                      <th scope="col">Quantidade</th>
                      <th scope="col">Porcentagem</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr>
                      <th scope="row">Tokens de Entrada</th>
                      <td>{formatNumber(stats.promptTokens)}</td>
                      <td>{calculatePercentage(stats.promptTokens, stats.totalTokens)}%</td>
                    </tr>
                    <tr>
                      <th scope="row">Tokens de Saída</th>
                      <td>{formatNumber(stats.completionTokens)}</td>
                      <td>{calculatePercentage(stats.completionTokens, stats.totalTokens)}%</td>
                    </tr>
                    <tr>
                      <th scope="row">Total de Tokens</th>
                      <td>{formatNumber(stats.totalTokens)}</td>
                      <td>100%</td>
                    </tr>
                    <tr>
                      <th scope="row">Mensagens</th>
                      <td>{formatNumber(stats.messageCount)}</td>
                      <td>
                        {stats.messageCount > 0 
                          ? `${Math.round(stats.totalTokens / stats.messageCount)} tokens/msg`
                          : '—'}
                      </td>
                    </tr>
                    <tr>
                      <th scope="row">Modelo Principal</th>
                      <td colSpan={2}>{stats.mostUsedModel || 'N/A'}</td>
                    </tr>
                  </tbody>
                </table>
              </section>

              {/* Estimativa de custo */}
              {estimatedCost && (
                <section className="token-stats-section">
                  <h3>Estimativa de Custo</h3>
                  <table className="token-stats-table token-stats-table--cost">
                    <thead>
                      <tr>
                        <th scope="col">Tipo</th>
                        <th scope="col">Valor (USD)</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr>
                        <th scope="row">Entrada</th>
                        <td>${estimatedCost.input.toFixed(4)}</td>
                      </tr>
                      <tr>
                        <th scope="row">Saída</th>
                        <td>${estimatedCost.output.toFixed(4)}</td>
                      </tr>
                      <tr className="token-stats-table__total">
                        <th scope="row">Total estimado</th>
                        <td>${(estimatedCost.input + estimatedCost.output).toFixed(4)}</td>
                      </tr>
                    </tbody>
                  </table>
                  <p className="token-stats-cost__note">
                    * Valores aproximados baseados em GPT-4
                  </p>
                </section>
              )}

              {/* Dicas */}
              <section className="token-stats-section">
                <h3>Dicas de Gerenciamento</h3>
                <ul className="token-stats-tips">
                  <li>💡 Mensagens mais antigas consomem mais contexto</li>
                  <li>🔄 Considere resumir conversas longas</li>
                  <li>🗑️ Remova mensagens desnecessárias do histórico</li>
                  <li>📊 Modelos diferentes têm limites diferentes</li>
                </ul>
              </section>
            </>
          )}
        </div>
      </div>
    </div>
  );
};
