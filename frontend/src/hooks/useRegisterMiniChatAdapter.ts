import { useEffect, useRef } from 'react';
import { registerMiniChatAdapter, type MiniChatAdapter } from '../store/miniChatStore';

/**
 * Regista um adaptador de mini-chat por aba. O objeto registado é um wrapper estável
 * (registo só depende de `tabId`); `prepare`/`send` delegam sempre para `adapterRef.current`,
 * evitando janelas em que o mapa fica sem adaptador quando o `useMemo` recria o objeto.
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
        if (!a) return Promise.resolve({ ok: false as const });
        return a.prepare();
      },
      send: (instruction, media, meta) => {
        const a = adapterRef.current;
        if (!a) return Promise.resolve();
        return a.send(instruction, media, meta);
      },
    };

    registerMiniChatAdapter(tabId, wrapper);
    return () => registerMiniChatAdapter(tabId, null);
  }, [tabId]);
}
