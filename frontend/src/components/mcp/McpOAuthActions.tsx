import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { InspectMCPOAuthInventory } from '@wailsjs/go/wailsapi/MCP';
import { useAuthStore } from '../../store/authStore';
import { useMCPStore } from '../../store/mcpStore';
import { ToolbarButton } from '../ui/Toolbar';
import { MenuButton } from '../layout/MenuButton';
import { McpOAuthInventory } from './McpOAuthInventory';

export function McpOAuthActions() {
  const user = useAuthStore((state) => state.user);
  // Never retain an inventory result or an open dialog across sessions.
  return user ? <SessionOAuthActions key={`${user.userId}:${user.sessionId}`} /> : null;
}

function SessionOAuthActions() {
  const { t } = useTranslation();
  const servers = useMCPStore((state) => state.servers);
  const [open, setOpen] = useState(false);
  const [revision, setRevision] = useState(0);
  const [pending, setPending] = useState(false);
  const request = useRef<{ servers: typeof servers; revision: number; promise: ReturnType<typeof InspectMCPOAuthInventory> }>();

  useEffect(() => {
    let active = true;
    if (!request.current || request.current.servers !== servers || request.current.revision !== revision) {
      request.current = { servers, revision, promise: Promise.resolve().then(() => InspectMCPOAuthInventory()) };
    }
    void request.current.promise.then((items) => {
      if (active) setPending((items || []).some((item) => item.kind === 'legacy' || item.kind === 'client_credentials'));
    }).catch(() => {
      if (active) setPending(false);
      // Unknown is not proof of completion. The advanced diagnostic stays
      // available and presents its existing safe error and retry-on-reopen.
    });
    return () => { active = false; };
  }, [servers, revision]);

  return <>
    {pending && <ToolbarButton label={t('mcp.inventory.migrate')} onClick={() => setOpen(true)} />}
    <MenuButton buttonLabel={t('mcp.inventory.advancedActions')} items={[
      { id: 'oauth-inventory', label: t('mcp.inventory.title'), onClick: () => setOpen(true) },
    ]} />
    <McpOAuthInventory isOpen={open} onClose={() => { setOpen(false); setRevision((value) => value + 1); }}
      onInventoryChanged={() => setRevision((value) => value + 1)} />
  </>;
}
