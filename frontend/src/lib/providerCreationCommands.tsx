import { useEffect, useRef, type RefObject } from 'react';
import { useTranslation } from 'react-i18next';
import { Menu } from '../components/menu';
import { useAnchoredContextMenu } from '../hooks/useAnchoredContextMenu';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { listCommandCatalog } from '../services/commandCatalog';
import { getModalRegistrySnapshot, subscribeModalRegistry } from './modalRegistry';
import { usePagePresentationCommands, type PagePresentationCommandID } from './commandPagePresentation';

export const PROVIDER_CREATION_COMMANDS = [
  'providers.create.open', 'providers.chatgpt.create.open',
  'providers.acp.create.open', 'providers.api.create.open',
] as const satisfies readonly PagePresentationCommandID[];
export type ProviderCreationKind = 'chatgpt' | 'acp' | 'api';

export function useProviderCreationCommands(options: {
  root: RefObject<HTMLElement>;
  pathname: string;
  ready: boolean;
  open(kind: ProviderCreationKind): void;
}) {
  const { t, i18n } = useTranslation();
  const { announce } = useAnnouncer();
  const buttonRef = useRef<HTMLButtonElement>(null);
  const menuHost = useRef<HTMLDivElement>(null);
  const latest = useRef(options);
  latest.current = options;
  const presentation = useRef({ t, announce, locale: i18n.language });
  presentation.current = { t, announce, locale: i18n.language };
  const sequence = useRef(0);
  const timer = useRef<ReturnType<typeof setTimeout>>();
  const intent = useRef<string>();
  const readContext = () => {
    const auth = useAuthStore.getState();
    const workspace = useWorkspaceStore.getState().workspace;
    const modal = getModalRegistrySnapshot();
    if (!auth.isAuthenticated || !auth.user || !workspace || modal.topID ||
        !latest.current.ready || latest.current.pathname !== '/settings/providers' ||
        !latest.current.root.current?.isConnected || !document.hasFocus()) return undefined;
    return JSON.stringify([auth.user.userId, auth.user.sessionId, workspace.id,
      workspace.activeTabId, latest.current.pathname, modal.generation]);
  };
  const currentContext = useRef(readContext);
  currentContext.current = readContext;
  const { menu, openForTrigger, closeMenu, onSelectItem } = useAnchoredContextMenu({
    restoreTriggerFocusOnSelect: false,
    restoreTriggerFocusOnDismiss: false,
  });
  const cancel = (restore = false) => {
    const mayRestore = restore && intent.current === currentContext.current();
    sequence.current++;
    clearTimeout(timer.current);
    intent.current = undefined;
    closeMenu();
    if (mayRestore) buttonRef.current?.focus();
  };
  const cancelRef = useRef(cancel);
  cancelRef.current = cancel;
  useEffect(() => {
    const changed = () => {
      if (intent.current && intent.current !== currentContext.current()) cancelRef.current();
    };
    const blur = () => cancelRef.current();
    const unsubscribe = [useAuthStore.subscribe(changed), useWorkspaceStore.subscribe(changed),
      subscribeModalRegistry(changed)];
    window.addEventListener('blur', blur);
    document.addEventListener('compositionstart', blur, true);
    return () => {
      sequence.current++;
      clearTimeout(timer.current);
      unsubscribe.forEach(off => off());
      window.removeEventListener('blur', blur);
      document.removeEventListener('compositionstart', blur, true);
    };
  }, []);
  useEffect(() => {
    if (intent.current && intent.current !== readContext()) cancel();
  });
  const { request } = usePagePresentationCommands({
    root: options.root,
    pathname: options.pathname,
    allowedCommands: PROVIDER_CREATION_COMMANDS,
    readTarget: () => currentContext.current(),
    isCurrent: () => Boolean(currentContext.current()),
    canOpen: () => Boolean(currentContext.current()),
    open: id => {
      const context = readContext();
      if (!context) return false;
      if (id !== 'providers.create.open') {
        cancel();
        const kind = id.split('.')[1] as ProviderCreationKind;
        latest.current.open(kind);
        return true;
      }
      if (menu.visible) { cancel(true); return true; }
      if (!buttonRef.current) return false;
      const focus = document.activeElement;
      const operation = ++sequence.current;
      intent.current = context;
      // Present the existing Menu immediately so Escape/Tab/outside clicks also
      // cancel the catalog wait. This is a native menu gesture, not a shortcut.
      openForTrigger(buttonRef.current, presentation.current.t('providers.creation.title'),
        [{ id: 'loading', label: presentation.current.t('providers.creation.cancelLoading'), action: () => cancel(true) }]);
      const valid = () => operation === sequence.current && intent.current === context &&
        context === currentContext.current();
      const fail = () => {
        if (!valid()) return;
        cancel(Boolean(menuHost.current?.contains(document.activeElement)));
        presentation.current.announce(presentation.current.t('commandPalette.error'), 'assertive');
      };
      clearTimeout(timer.current);
      timer.current = setTimeout(fail, 5000);
      void Promise.resolve().then(() => listCommandCatalog({ locale: presentation.current.locale, source: 'palette' })).then(catalog => {
        if (!valid()) return;
        clearTimeout(timer.current);
        if ((document.activeElement !== focus && !menuHost.current?.contains(document.activeElement)) || !buttonRef.current) { cancel(); return; }
        const choices = PROVIDER_CREATION_COMMANDS.slice(1).map(commandID => {
          const item = catalog.find(candidate => candidate.id === commandID);
          return {
            id: commandID, label: item?.name || presentation.current.t('providers.creation.' + commandID.split('.')[1]),
            disabled: !item?.available,
            action: () => { if (valid()) request(commandID); },
          };
        });
        openForTrigger(buttonRef.current, presentation.current.t('providers.creation.title'), choices);
      }).catch(fail);
      return true;
    },
  });
  return {
    buttonRef,
    isOpen: menu.visible,
    requestOpen: () => request('providers.create.open'),
    menu: <div ref={menuHost} onKeyDownCapture={event => {
      if (event.key !== 'Tab' || !menu.visible) return;
      // Native menu dismissal: restore the anchor before the browser advances
      // focus, without canceling Tab or Shift+Tab. No application shortcut.
      event.stopPropagation();
      cancel(true);
    }}><Menu key={menu.items[0]?.id === 'loading' ? 'loading' : 'choices'} {...menu} restoreFocusOnClose={false} onSelect={onSelectItem}
      onClose={() => cancel(true)} /></div>,
  };
}
