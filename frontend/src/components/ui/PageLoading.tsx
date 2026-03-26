import { LoadingOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import './PageLoading.css';

interface PageLoadingProps {
  message?: string;
  className?: string;
}

export function PageLoading({ message, className = '' }: PageLoadingProps) {
  const { t } = useTranslation();
  return (
    <div className={`page-loading ${className}`} role="status" aria-busy="true">
      <LoadingOutlined spin className="page-loading__spinner" aria-hidden="true" />
      <span className="page-loading__text">{message || t('common.loading', 'Carregando...')}</span>
    </div>
  );
}
