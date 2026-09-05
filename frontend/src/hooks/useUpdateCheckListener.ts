import { useTranslation } from 'react-i18next';
import { useUIStore } from '../store/uiStore';
import { useWailsEvent } from './useWails';

export const UPDATE_CHECK_ERROR_EVENT = 'update:check-error';

/**
 * Exibe o erro de verificação automática sem bloquear o uso do app.
 * O backend agrega falhas repetidas; o toast usa o announcer global do store.
 */
export function useUpdateCheckListener() {
  const { t } = useTranslation();
  const addToast = useUIStore((state) => state.addToast);

  useWailsEvent(UPDATE_CHECK_ERROR_EVENT, () => {
    addToast(t('app.updater.checkError'), 'warning', 10000);
  });
}
