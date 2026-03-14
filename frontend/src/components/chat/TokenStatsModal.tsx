import React, { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { GetConversationTokenStats } from '@wailsjs/go/main/App';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { Modal } from '../ui/Modal';
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
  const { t } = useTranslation();
  const [stats, setStats] = useState<TokenStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

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
        setError(t('tokenStats.loadError'));
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
  }, [conversationId, isOpen, t]);

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
    <Modal isOpen={isOpen} onClose={onClose} title={t('tokenStats.title')} size="md">
      <div className="token-stats-modal__content">
        {loading && (
          <div className="token-stats-modal__loading">
            <span>{t('tokenStats.loading')}</span>
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
              <h3>{t('tokenStats.contextUsage')}</h3>
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
                      <span>🔴 {t('tokenStats.contextCritical')}</span>
                    ) : (
                      <span>🟡 {t('tokenStats.contextWarning')}</span>
                    )}
                  </div>
                )}
              </div>
            </section>

            {/* Detalhamento de tokens */}
            <section className="token-stats-section">
              <h3>{t('tokenStats.breakdown')}</h3>
              <table className="token-stats-table">
                <thead>
                  <tr>
                    <th scope="col">{t('tokenStats.category')}</th>
                    <th scope="col">{t('tokenStats.quantity')}</th>
                    <th scope="col">{t('tokenStats.percentage')}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <th scope="row">{t('tokenStats.inputTokens')}</th>
                    <td>{formatNumber(stats.promptTokens)}</td>
                    <td>{calculatePercentage(stats.promptTokens, stats.totalTokens)}%</td>
                  </tr>
                  <tr>
                    <th scope="row">{t('tokenStats.outputTokens')}</th>
                    <td>{formatNumber(stats.completionTokens)}</td>
                    <td>{calculatePercentage(stats.completionTokens, stats.totalTokens)}%</td>
                  </tr>
                  <tr>
                    <th scope="row">{t('tokenStats.totalTokens')}</th>
                    <td>{formatNumber(stats.totalTokens)}</td>
                    <td>100%</td>
                  </tr>
                  <tr>
                    <th scope="row">{t('tokenStats.messages')}</th>
                    <td>{formatNumber(stats.messageCount)}</td>
                    <td>
                      {stats.messageCount > 0
                        ? `${Math.round(stats.totalTokens / stats.messageCount)} ${t('tokenStats.tokensPerMsg')}`
                        : '—'}
                    </td>
                  </tr>
                  <tr>
                    <th scope="row">{t('tokenStats.mainModel')}</th>
                    <td colSpan={2}>{stats.mostUsedModel || 'N/A'}</td>
                  </tr>
                </tbody>
              </table>
            </section>

            {/* Estimativa de custo */}
            {estimatedCost && (
              <section className="token-stats-section">
                <h3>{t('tokenStats.costEstimate')}</h3>
                <table className="token-stats-table token-stats-table--cost">
                  <thead>
                    <tr>
                      <th scope="col">{t('tokenStats.type')}</th>
                      <th scope="col">{t('tokenStats.valueUSD')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr>
                      <th scope="row">{t('tokenStats.input')}</th>
                      <td>${estimatedCost.input.toFixed(4)}</td>
                    </tr>
                    <tr>
                      <th scope="row">{t('tokenStats.output')}</th>
                      <td>${estimatedCost.output.toFixed(4)}</td>
                    </tr>
                    <tr className="token-stats-table__total">
                      <th scope="row">{t('tokenStats.totalEstimated')}</th>
                      <td>${(estimatedCost.input + estimatedCost.output).toFixed(4)}</td>
                    </tr>
                  </tbody>
                </table>
                <p className="token-stats-cost__note">
                  {t('tokenStats.costDisclaimer')}
                </p>
              </section>
            )}

            {/* Dicas */}
            <section className="token-stats-section">
              <h3>{t('tokenStats.managementTips')}</h3>
              <ul className="token-stats-tips">
                <li>{t('tokenStats.tip1')}</li>
                <li>{t('tokenStats.tip2')}</li>
                <li>{t('tokenStats.tip3')}</li>
                <li>{t('tokenStats.tip4')}</li>
              </ul>
            </section>
          </>
        )}
      </div>
    </Modal>
  );
};

