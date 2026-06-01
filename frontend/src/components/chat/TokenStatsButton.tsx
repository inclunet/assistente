import { logger } from '../../utils/logger';
import React, { useState, useEffect } from 'react';
import { BarChartOutlined, LoadingOutlined, WarningOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { GetConversationTokenStats } from '@wailsjs/go/app/App';
import { EventsOn } from '@wailsjs/runtime/runtime';
import './TokenStatsButton.css';

interface TokenStats {
  conversationId: string;
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

interface TokenStatsButtonProps {
  conversationId?: string;
  onOpenModal: () => void;
}

export const TokenStatsButton: React.FC<TokenStatsButtonProps> = ({
  conversationId,
  onOpenModal,
}) => {
  const { t } = useTranslation();
  const [stats, setStats] = useState<TokenStats | null>(null);
  const [isLoading, setIsLoading] = useState(false);

  useEffect(() => {
    if (!conversationId) {
      setStats(null);
      return;
    }

    // Função para carregar stats do backend
    const loadStats = () => {
      setIsLoading(true);
      GetConversationTokenStats(conversationId)
        .then((data) => {
          setStats(data);
        })
        .catch((error) => {
          logger.error('[TokenStatsButton] Erro ao carregar stats:', error);
          // Define stats padrão em caso de erro
          setStats({
            conversationId,
            promptTokens: 0,
            completionTokens: 0,
            totalTokens: 0,
            messageCount: 0,
            mostUsedModel: '',
            contextUsage: 0,
            contextLimit: 128000,
            isNearLimit: false,
            isCritical: false,
          });
        })
        .finally(() => setIsLoading(false));
    };

    // Carrega stats iniciais
    loadStats();

    // Escuta atualizações de tokens (emitido após streaming)
    const unsubscribeTokens = EventsOn('chat:token_stats', (data: TokenStats) => {
      if (data.conversationId === conversationId) {
        setStats(data);
      }
    });

    // Escuta atualizações em TEMPO REAL durante execução (após cada tool call)
    const unsubscribeRealtime = EventsOn('chat:token_stats_update', (data: TokenStats) => {
      if (data.conversationId === conversationId) {
        setStats(data);
      }
    });

    // Escuta quando mensagens do usuário são adicionadas (antes do streaming)
    const unsubscribeMessages = EventsOn('chat:messages_ready', (data: unknown) => {
      const eventData = data as { conversationId?: string };
      if (eventData.conversationId === conversationId) {
        loadStats();
      }
    });

    // Escuta quando o streaming termina (garantia extra de atualização)
    const unsubscribeDone = EventsOn('chat:done', (data: unknown) => {
      const eventData = data as { conversationId?: string };
      if (eventData.conversationId === conversationId) {
        loadStats();
      }
    });

    return () => {
      unsubscribeTokens();
      unsubscribeRealtime();
      unsubscribeMessages();
      unsubscribeDone();
    };
  }, [conversationId]);

  // Não renderiza se não houver conversationId
  if (!conversationId) {
    return null;
  }

  // Mostra loading ou valores padrão enquanto carrega
  if (isLoading || !stats) {
    return (
      <button
        className="token-stats-button token-stats-button--loading"
        disabled
        aria-label={t('chat.loadingTokenStats')}
      >
        <span className="token-stats-button__icon" aria-hidden="true"><LoadingOutlined spin /></span>
        <span className="token-stats-button__text">...</span>
      </button>
    );
  }

  const formatNumber = (num: number): string => {
    if (num >= 1000000) {
      return `${(num / 1000000).toFixed(1)}M`;
    }
    if (num >= 1000) {
      return `${(num / 1000).toFixed(1)}K`;
    }
    return num.toString();
  };

  const getStatusColor = (): string => {
    if (stats.isCritical) return 'critical';
    if (stats.isNearLimit) return 'warning';
    return 'normal';
  };

  const getStatusIcon = () => {
    if (stats.isCritical) return <WarningOutlined style={{ color: 'var(--color-danger)' }} />;
    if (stats.isNearLimit) return <WarningOutlined style={{ color: 'var(--color-warning)' }} />;
    return <BarChartOutlined />;
  };

  // Se não houver limite de contexto configurado, mostra apenas o total
  const hasContextLimit = stats.contextLimit > 0;
  const ariaLabel = hasContextLimit
    ? `${formatNumber(stats.totalTokens)} de ${formatNumber(stats.contextLimit)} tokens`
    : `${formatNumber(stats.totalTokens)} tokens`;

  return (
    <button
      className={`token-stats-button token-stats-button--${getStatusColor()}`}
      onClick={onOpenModal}
      aria-label={`${ariaLabel}. Consumo de contexto: ${stats.contextUsage.toFixed(1)}% ${t('chat.tokenDetailsShortcut')}`}
    >
      <span className="token-stats-button__icon" aria-hidden="true">
        {getStatusIcon()}
      </span>
      <span className="token-stats-button__text">
        {formatNumber(stats.totalTokens)}
        {hasContextLimit && (
          <>
            <span className="token-stats-button__separator">/</span>
            {formatNumber(stats.contextLimit)}
          </>
        )}
      </span>
      {hasContextLimit && (
        <span className="token-stats-button__context-badge" aria-label={`${stats.contextUsage.toFixed(1)}% do contexto consumido`}>
          {stats.contextUsage.toFixed(1)}%
        </span>
      )}
    </button>
  );
};

