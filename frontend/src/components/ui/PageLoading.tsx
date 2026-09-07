import { useEffect, useMemo } from 'react';
import { LoadingOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { useOptionalWorkspacePanel } from '../workspace/WorkspacePanelContext';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { buildVoiceAccessibilityOriginFromTab } from '../../services/voiceAccessibility/types';
import './PageLoading.css';

interface PageLoadingProps {
  message?: string;
  className?: string;
}

export function PageLoading({ message, className = '' }: PageLoadingProps) {
  const { t } = useTranslation();
  const { announceRequest } = useAnnouncer();
  const workspacePanel = useOptionalWorkspacePanel();
  const workspace = useWorkspaceStore((state) => state.workspace);
  const loadingMessage = message || t('common.loading', 'Carregando...');
  const accessibilityOrigin = useMemo(() => (
    workspacePanel ? buildVoiceAccessibilityOriginFromTab(workspacePanel.tab, workspace) : undefined
  ), [workspace, workspacePanel]);

  useEffect(() => {
    announceRequest({
      message: loadingMessage,
      origin: accessibilityOrigin,
      eventType: 'progress',
    });
  }, [accessibilityOrigin, announceRequest, loadingMessage]);

  return (
    <div className={`page-loading ${className}`} aria-busy="true">
      <LoadingOutlined spin className="page-loading__spinner" aria-hidden="true" />
      <span className="page-loading__text">{loadingMessage}</span>
    </div>
  );
}
