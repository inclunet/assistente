import { useEffect } from 'react';
import { LoadingOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import './PageLoading.css';

interface PageLoadingProps {
  message?: string;
  className?: string;
}

export function PageLoading({ message, className = '' }: PageLoadingProps) {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const loadingMessage = message || t('common.loading', 'Carregando...');

  useEffect(() => {
    announce(loadingMessage);
  }, [announce, loadingMessage]);

  return (
    <div className={`page-loading ${className}`} aria-busy="true">
      <LoadingOutlined spin className="page-loading__spinner" aria-hidden="true" />
      <span className="page-loading__text">{loadingMessage}</span>
    </div>
  );
}
