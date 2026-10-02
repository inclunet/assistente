import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { InspectMCPOAuthInventory } from '@wailsjs/go/wailsapi/MCP';
import type { mcp } from '../../../wailsjs/go/models';
import { Modal } from '../ui/Modal';
import { Button } from '../ui/Button';
import { DialogActions } from '../ui/DialogActions';
import { useAnnouncer } from '../../hooks/useAnnouncer';

export function McpOAuthInventory({ isOpen, onClose }: { isOpen: boolean; onClose: () => void }) {
  const { t } = useTranslation();
  return (
    <Modal isOpen={isOpen} onClose={onClose} title={t('mcp.inventory.title')} readingMode size="lg">
      {isOpen && <OAuthInventoryContent />}
      <DialogActions primary={<Button onClick={onClose}>{t('common.close')}</Button>} />
    </Modal>
  );
}

// Only the query content is mounted per opening. Modal stays mounted so its
// true -> false transition restores the page's registered default focus.
function OAuthInventoryContent() {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const [items, setItems] = useState<mcp.OAuthInventoryItem[] | null>(null);
  const [failed, setFailed] = useState(false);
  const request = useRef<Promise<mcp.OAuthInventoryItem[]> | null>(null);

  useEffect(() => {
    let active = true;
    if (!request.current) {
      request.current = Promise.resolve().then(() => InspectMCPOAuthInventory());
      announce(t('mcp.inventory.loading'));
    }
    void request.current.then((result) => {
      if (!active) return;
      setItems(result || []);
      announce(t('mcp.inventory.loaded'));
    }).catch(() => {
      if (active) {
        setFailed(true);
        announce(t('mcp.inventory.failed'), 'assertive');
      }
    });
    return () => { active = false; };
  }, [announce, t]);

  return (
    <>
      <p>{t('mcp.inventory.description')}</p>
      {failed ? <p>{t('mcp.inventory.failed')}</p> : items === null ? (
        <p>{t('mcp.inventory.loading')}</p>
      ) : items.length === 0 ? <p>{t('mcp.inventory.empty')}</p> : (
        <ul>
          {items.map((item) => (
            <li key={item.id}>
              <h2>{item.name}</h2>
              <p>{t(`mcp.inventory.kinds.${item.kind}`)}</p>
              {item.issues.length === 0 ? <p>{t('mcp.inventory.noIssues')}</p> : (
                <ul>{item.issues.map((issue) => <li key={issue}>{t(`mcp.inventory.issues.${issue}`)}</li>)}</ul>
              )}
            </li>
          ))}
        </ul>
      )}
    </>
  );
}
