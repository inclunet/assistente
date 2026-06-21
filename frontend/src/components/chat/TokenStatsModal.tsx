import { logger } from '../../utils/logger';
import React, { useState, useEffect } from 'react';
import { WarningOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { GetConversationTokenStats } from '@wailsjs/go/app/App';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { Modal } from '../ui/Modal';
import { Tabs, TabList, Tab, TabPanel } from '../ui/tabs';
import './TokenStatsModal.css';

interface ToolBreakdownEntry {
  toolName: string;
  callCount: number;
  totalPromptTokens: number;
  totalCompletionTokens: number;
  totalTokens: number;
}

interface TokenStats {
  totalTokens: number;
  cacheReadTokens?: number;
  cacheWriteTokens?: number;
  cacheMissTokens?: number;
  cacheHitRate?: number;
  cacheTokensReported?: boolean;
  promptCacheEnabled?: boolean;
  contextTokens: number;
  promptTokens: number;
  completionTokens: number;
  contextLimit: number;
  contextUsage: number;
  isNearLimit: boolean;
  isCritical: boolean;
  messageCount: number;
  mostUsedModel: string;
  systemPromptEstimatedTokens: number;
  summaryTokens: number;
  messagesInContextTokens: number;
  messagesOutOfContextTokens: number;
  messagesInContextCount: number;
  messagesOutOfContextCount: number;
  toolsUsedCount: number;
  toolBreakdown: ToolBreakdownEntry[];
}

interface TokenStatsModalProps {
  conversationId: string;
  isOpen: boolean;
  onClose: () => void;
}

export const TokenStatsModal: React.FC<TokenStatsModalProps> = ({
  conversationId,
  isOpen,
  onClose,
}) => {
  const { t, i18n } = useTranslation();
  const [stats, setStats] = useState<TokenStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState('overview');

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
        logger.error('[TokenStatsModal] Erro ao carregar estatísticas:', err);
        setError(t('tokenStats.loadError'));
      } finally {
        setLoading(false);
      }
    };

    loadStats();

    // Escuta atualizações em tempo real
    const unsubscribe = EventsOn('chat:token_stats', (data: TokenStats & { conversationId: string }) => {
      if (data.conversationId === conversationId) {
        setStats(data);
      }
    });

    return () => unsubscribe();
  }, [conversationId, isOpen, t]);

  const formatNumber = (num: number): string => {
    return (Number.isFinite(num) ? num : 0).toLocaleString(i18n.language);
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

  const getCacheClassifiedTokens = (value: TokenStats): number =>
    (value.cacheReadTokens ?? 0) + (value.cacheWriteTokens ?? 0) + (value.cacheMissTokens ?? 0);

  const formatCachePercentage = (value: number, total: number): string =>
    stats?.cacheTokensReported ? `${calculatePercentage(value, total)}%` : '—';

  const formatCacheHitRate = (): string =>
    stats?.cacheTokensReported ? `${(stats.cacheHitRate ?? 0).toFixed(1)}%` : '—';

  const estimatedCost = stats ? {
    input: (Math.max(0, stats.promptTokens - (stats.cacheReadTokens ?? 0)) / 1000000) * 0.5, // estimativa genérica
    output: (stats.completionTokens / 1000000) * 1.5, // $1.50 por 1M tokens
  } : null;

  return (
    <Modal isOpen={isOpen} onClose={onClose} title={t('tokenStats.title')} size="md" readingMode>
      <div className="token-stats-modal__content">
        {loading && (
          <div className="token-stats-modal__loading">
            <span>{t('tokenStats.loading')}</span>
          </div>
        )}

        {error && (
          <div className="token-stats-modal__error">
            <span><WarningOutlined aria-hidden="true" /> {error}</span>
          </div>
        )}

        {!loading && !error && stats && (
          <Tabs value={activeTab} onValueChange={setActiveTab}>
            <TabList ariaLabel={t('tokenStats.title')}>
              <Tab value="overview">{t('tokenStats.tabOverview')}</Tab>
              <Tab value="cache">{t('tokenStats.tabPromptCache')}</Tab>
              <Tab value="context">{t('tokenStats.tabContextDetails')}</Tab>
              <Tab value="tools">{t('tokenStats.tabToolCalling')}</Tab>
              <Tab value="loop">{t('tokenStats.tabAgenticLoop')}</Tab>
            </TabList>

            {/* TAB: Overview */}
            <TabPanel value="overview">
              <section className="token-stats-section">
                <h3>{t('tokenStats.contextUsage')}</h3>
                <div className="token-stats-context">
                  <div className="token-stats-context__numbers">
                    <span className="token-stats-context__current">
                      {formatNumber(stats.contextTokens)}
                    </span>
                    <span className="token-stats-context__separator">/</span>
                    <span className="token-stats-context__limit">
                      {formatNumber(stats.contextLimit)}
                    </span>
                    <span className="token-stats-context__percentage">
                      ({stats.contextUsage.toFixed(1)}%)
                    </span>
                  </div>
                  <p className="token-stats-cost__note">
                    {t('tokenStats.currentContextNote')}
                  </p>
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
                        <span><WarningOutlined aria-hidden="true" style={{ color: 'var(--color-danger)' }} /> {t('tokenStats.contextCritical')}</span>
                      ) : (
                        <span><WarningOutlined aria-hidden="true" style={{ color: 'var(--color-warning)' }} /> {t('tokenStats.contextWarning')}</span>
                      )}
                    </div>
                  )}
                </div>
              </section>

              <section className="token-stats-section">
                <h3>{t('tokenStats.breakdown')}</h3>
                <p className="token-stats-cost__note">
                  {t('tokenStats.cumulativeNote')}
                </p>
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
                    {stats.cacheTokensReported
                      ? t('tokenStats.costDisclaimerWithCache')
                      : t('tokenStats.costDisclaimer')}
                  </p>
                </section>
              )}

              <section className="token-stats-section">
                <h3>{t('tokenStats.managementTips')}</h3>
                <ul className="token-stats-tips">
                  <li>{t('tokenStats.tip1')}</li>
                  <li>{t('tokenStats.tip2')}</li>
                  <li>{t('tokenStats.tip3')}</li>
                  <li>{t('tokenStats.tip4')}</li>
                </ul>
              </section>
            </TabPanel>

            {/* TAB: Prompt Cache */}
            <TabPanel value="cache">
              <section className="token-stats-section">
                <h3>{t('tokenStats.promptCache')}</h3>
                {stats.promptCacheEnabled === false && (
                  <p className="token-stats-info">
                    {t('tokenStats.cacheDisabledNote')}
                  </p>
                )}
                <p className="token-stats-cost__note">
                  {stats.cacheTokensReported
                    ? t('tokenStats.cacheReportedNote')
                    : t('tokenStats.cacheUnavailableNote')}
                </p>
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
                      <th scope="row">{t('tokenStats.cacheReadTokens')}</th>
                      <td>{formatNumber(stats.cacheReadTokens ?? 0)}</td>
                      <td>{formatCachePercentage(stats.cacheReadTokens ?? 0, getCacheClassifiedTokens(stats))}</td>
                    </tr>
                    <tr>
                      <th scope="row">{t('tokenStats.cacheWriteTokens')}</th>
                      <td>{formatNumber(stats.cacheWriteTokens ?? 0)}</td>
                      <td>{formatCachePercentage(stats.cacheWriteTokens ?? 0, getCacheClassifiedTokens(stats))}</td>
                    </tr>
                    <tr>
                      <th scope="row">{t('tokenStats.cacheMissTokens')}</th>
                      <td>{formatNumber(stats.cacheMissTokens ?? 0)}</td>
                      <td>{formatCachePercentage(stats.cacheMissTokens ?? 0, getCacheClassifiedTokens(stats))}</td>
                    </tr>
                    <tr className="token-stats-table__total">
                      <th scope="row">{t('tokenStats.cacheHitRate')}</th>
                      <td>{formatCacheHitRate()}</td>
                      <td>—</td>
                    </tr>
                  </tbody>
                </table>
              </section>
            </TabPanel>

            {/* TAB: Context Details */}
            <TabPanel value="context">
              <section className="token-stats-section">
                <h3>{t('tokenStats.contextComposition')}</h3>
                <table className="token-stats-table">
                  <thead>
                    <tr>
                      <th scope="col">{t('tokenStats.component')}</th>
                      <th scope="col">{t('tokenStats.tokenCount')}</th>
                      <th scope="col">{t('tokenStats.percentage')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr>
                      <th scope="row">{t('tokenStats.systemPrompt')}</th>
                      <td>{formatNumber(stats.systemPromptEstimatedTokens)}</td>
                      <td>{calculatePercentage(stats.systemPromptEstimatedTokens, stats.totalTokens)}%</td>
                    </tr>
                    <tr>
                      <th scope="row">{t('tokenStats.summary')}</th>
                      <td>{formatNumber(stats.summaryTokens)}</td>
                      <td>{calculatePercentage(stats.summaryTokens, stats.totalTokens)}%</td>
                    </tr>
                    <tr>
                      <th scope="row">{t('tokenStats.messagesInContext')}</th>
                      <td>{formatNumber(stats.messagesInContextTokens)}</td>
                      <td>{calculatePercentage(stats.messagesInContextTokens, stats.totalTokens)}%</td>
                    </tr>
                    <tr>
                      <th scope="row">{t('tokenStats.messagesOutOfContext')}</th>
                      <td>{formatNumber(stats.messagesOutOfContextTokens)}</td>
                      <td>{calculatePercentage(stats.messagesOutOfContextTokens, stats.totalTokens)}%</td>
                    </tr>
                  </tbody>
                </table>
              </section>

              <section className="token-stats-section">
                <h3>{t('tokenStats.messageBreakdown')}</h3>
                <table className="token-stats-table">
                  <thead>
                    <tr>
                      <th scope="col">{t('tokenStats.type')}</th>
                      <th scope="col">{t('tokenStats.count')}</th>
                      <th scope="col">{t('tokenStats.totalTokens')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr>
                      <th scope="row">{t('tokenStats.inContext')}</th>
                      <td>{formatNumber(stats.messagesInContextCount)}</td>
                      <td>{formatNumber(stats.messagesInContextTokens)}</td>
                    </tr>
                    <tr>
                      <th scope="row">{t('tokenStats.outOfContext')}</th>
                      <td>{formatNumber(stats.messagesOutOfContextCount)}</td>
                      <td>{formatNumber(stats.messagesOutOfContextTokens)}</td>
                    </tr>
                  </tbody>
                </table>
              </section>
            </TabPanel>

            {/* TAB: Tool Calling */}
            <TabPanel value="tools">
              {stats.toolsUsedCount > 0 ? (
                <section className="token-stats-section">
                  <h3>{t('tokenStats.toolUsageDetails')}</h3>
                  <p className="token-stats-summary">
                    {t('tokenStats.toolsUsed', { count: stats.toolsUsedCount })}
                  </p>
                  <table className="token-stats-table">
                    <thead>
                      <tr>
                        <th scope="col">{t('tokenStats.toolName')}</th>
                        <th scope="col">{t('tokenStats.callCount')}</th>
                        <th scope="col">{t('tokenStats.inputTokens')}</th>
                        <th scope="col">{t('tokenStats.outputTokens')}</th>
                        <th scope="col">{t('tokenStats.totalTokens')}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {stats.toolBreakdown.map((tool) => (
                        <tr key={tool.toolName}>
                          <th scope="row">{tool.toolName}</th>
                          <td>{formatNumber(tool.callCount)}</td>
                          <td>{formatNumber(tool.totalPromptTokens)}</td>
                          <td>{formatNumber(tool.totalCompletionTokens)}</td>
                          <td>{formatNumber(tool.totalTokens)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </section>
              ) : (
                <section className="token-stats-section">
                  <p className="token-stats-empty">
                    {t('tokenStats.noToolsUsed')}
                  </p>
                </section>
              )}
            </TabPanel>

            {/* TAB: Agentic Loop */}
            <TabPanel value="loop">
              <section className="token-stats-section">
                <h3>{t('tokenStats.agenticLoopStats')}</h3>
                <p className="token-stats-info">
                  {t('tokenStats.agenticLoopPlaceholder')}
                </p>
              </section>
            </TabPanel>
          </Tabs>
        )}
      </div>
    </Modal>
  );
};

