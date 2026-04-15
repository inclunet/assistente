import { useEffect, useRef } from 'react';
import { registerMiniChatAdapter, type MiniChatAdapter } from '../store/miniChatStore';

/**
 * Regista o adaptador de mini-chat para a aba do workspace.
 * Usa um wrapper estável para o registo não depender da identidade do objeto `adapter` a cada render.
 */
export function useRegisterMiniChatAdapter(
  tabId: string | undefined,
  adapter: MiniChatAdapter | null,
) {
  const adapterRef = useRef<MiniChatAdapter | null>(null);
  adapterRef.current = adapter;

  useEffect(() => {
    if (!tabId) return;

    if (!adapterRef.current) {
      registerMiniChatAdapter(tabId, null);
      return;
    }

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
  }, [tabId, adapter]);
}
