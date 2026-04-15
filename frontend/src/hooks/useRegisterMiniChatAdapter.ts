import { useEffect, useRef } from 'react';
import i18next from 'i18next';
import { registerMiniChatAdapter, type MiniChatAdapter } from '../store/miniChatStore';

/**
 * Regista um adaptador de mini-chat por aba. O mapa guarda sempre um **wrapper** estável
 * (efeito só depende de `tabId`); `prepare`/`send` delegam para `adapterRef.current`.
 *
 * Importante: quando o `useMemo` do painel ainda devolve `null` (ex.: tasklist a carregar),
 * **não** removemos o registo — caso contrário o mini-chat aberto ficava sem adaptador e o
 * envio falhava em silêncio.
 */
export function useRegisterMiniChatAdapter(
  tabId: string | undefined,
  adapter: MiniChatAdapter | null,
) {
  const adapterRef = useRef<MiniChatAdapter | null>(null);
  adapterRef.current = adapter;

  useEffect(() => {
    if (!tabId) return;

    const wrapper: MiniChatAdapter = {
      prepare: () => {
        const a = adapterRef.current;
        if (!a) {
          return Promise.resolve({
            ok: false as const,
            message: i18next.t('workspace.miniChat.panelLoading'),
          });
        }
        return a.prepare();
      },
      send: (instruction, media, meta, session) => {
        const a = adapterRef.current;
        if (!a) {
          return Promise.reject(
            new Error(
              i18next.t('workspace.miniChat.adapterUnavailable', {
                defaultValue: 'Mini-chat: adaptador do painel indisponível. Feche e abra o mini-chat.',
              }),
            ),
          );
        }
        return a.send(instruction, media, meta, session);
      },
    };

    registerMiniChatAdapter(tabId, wrapper);
    return () => registerMiniChatAdapter(tabId, null);
  }, [tabId]);
}
