import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { NeedsWelcomeWizard, RunWelcomeWizard } from '@wailsjs/go/wailsapi/Welcome';
import { Modal } from '../ui/Modal';
import { ProviderForm } from './ProviderForm';
import { useUIStore } from '../../store/uiStore';
import { waitForWailsBridge } from '../../lib/waitForWailsBridge';
import { logger } from '../../utils/logger';

// Mounted with the authenticated session as its key. The draft and async
// results belong to this instance, never to a subsequently logged-in user.
export function WelcomeSetup() {
  const { t } = useTranslation();
  const addToast = useUIStore(s => s.addToast);
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const preparation = useRef<Promise<'ready' | 'cancelled' | 'configured'> | null>(null);
  const active = useRef(false);
  const presented = useRef(false);

  useEffect(() => {
    active.current = true;
    let current = true;
    // Reuse the operation across Strict Mode's effect replay; changing the
    // language must not start another master-password questionnaire.
    preparation.current ??= (async () => {
      await waitForWailsBridge();
      if (!active.current) return 'cancelled';
      const needed = await NeedsWelcomeWizard();
      if (!active.current) return 'cancelled';
      if (!needed) return 'configured';
      const prepared = await RunWelcomeWizard();
      if (!active.current || !prepared) return 'cancelled';
      return await NeedsWelcomeWizard() ? 'ready' : 'configured';
    })();
    void preparation.current.then(result => {
      if (!current || presented.current) return;
      presented.current = true;
      if (result === 'ready') setOpen(true);
      if (result === 'cancelled') addToast(t('app.wizard.cancelled'), 'warning');
    }).catch(error => {
      if (!current || presented.current) return;
      presented.current = true;
      logger.error('[Welcome] Falha ao preparar configuração inicial:', error);
      addToast(t('app.wizard.error'), 'error');
    });
    return () => { current = false; active.current = false; };
  }, [addToast, t]);

  const cancel = () => {
    if (busy) return;
    setOpen(false);
    addToast(t('app.wizard.cancelled'), 'warning');
  };

  return <Modal isOpen={open} title={t('app.wizard.providerTitle')} onClose={cancel} allowClose={!busy} size="lg">
    <ProviderForm kind="api" onBusyChange={setBusy} onCancel={cancel} onSave={() => {
      if (!active.current) return;
      setOpen(false);
      addToast(t('app.wizard.success'), 'success', 5000);
    }} />
  </Modal>;
}
