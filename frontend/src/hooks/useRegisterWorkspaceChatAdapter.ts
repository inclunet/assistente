import { useEffect, useRef } from 'react';
import i18next from 'i18next';
import {
  registerWorkspaceChatModalAdapter,
  type WorkspaceChatModalAdapter,
} from '../store/workspaceChatModalStore';

/**
 * Regista um adaptador de modal de chat por aba. O mapa guarda sempre um
 * wrapper estável (efeito só depende de `tabId`); `prepare`/`send` delegam
 * para `adapterRef.current`.
 *
 * Importante: quando o `useMemo` do painel ainda devolve `null` (ex.: tasklist
 * a carregar), não removemos o registo. Caso contrário, o modal aberto ficava
 * sem adaptador e o envio falhava em silêncio.
 */
export function useRegisterWorkspaceChatAdapter(
  tabId: string | undefined,
  adapter: WorkspaceChatModalAdapter | null,
) {
  const adapterRef = useRef<WorkspaceChatModalAdapter | null>(null);
  adapterRef.current = adapter;

  useEffect(() => {
    if (!tabId) return;

    const wrapper: WorkspaceChatModalAdapter = {
      prepare: () => {
        const a = adapterRef.current;
        if (!a) {
          return Promise.resolve({
            ok: false as const,
            message: i18next.t('workspace.chatModal.panelLoading'),
          });
        }
        return a.prepare();
      },
      send: (instruction, media, meta, session) => {
        const a = adapterRef.current;
        if (!a) {
          return Promise.reject(
            new Error(
              i18next.t('workspace.chatModal.adapterUnavailable', {
                defaultValue:
                  'Chat modal: adaptador do painel indisponível. Feche e abra o chat novamente.',
              }),
            ),
          );
        }
        return a.send(instruction, media, meta, session);
      },
    };

    registerWorkspaceChatModalAdapter(tabId, wrapper);
    return () => registerWorkspaceChatModalAdapter(tabId, null);
  }, [tabId]);
}
