import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import type { ExternalUIDestination, ExternalUIConnectionService } from './externalUIConnection';
import { createWailsExternalUIConnectionService } from './externalUIConnectionWails';

interface ExternalUIConnectionContextValue {
  service: ExternalUIConnectionService | null;
  target: ExternalUIDestination | null;
  setTarget(target: ExternalUIDestination | null): void;
}

const emptyConnection: ExternalUIConnectionContextValue = {
  service: null, target: null, setTarget: () => undefined,
};
const ExternalUIConnectionContext = createContext(emptyConnection);

export function useExternalUIConnection(): ExternalUIConnectionContextValue {
  return useContext(ExternalUIConnectionContext);
}

/** Uma instância por interface autenticada. O destino vem do registro de
 * surfaces do produto; o provider nunca inventa uma surface ou concede acesso. */
export function ExternalUIConnectionProvider({ children, createService = createWailsExternalUIConnectionService }: {
  children: ReactNode;
  createService?: () => ExternalUIConnectionService;
}) {
  const identity = useAuthStore(state => state.isAuthenticated && state.user
    ? JSON.stringify([state.user.userId, state.user.sessionId]) : null);
  const workspaceID = useWorkspaceStore(state => state.workspace?.id ?? null);
  const ownerKey = identity && workspaceID ? JSON.stringify([identity, workspaceID]) : null;
  // Voltar ao mesmo workspace não ressuscita callbacks ou destinos da montagem anterior.
  const lifetime = useMemo(() => ({ ownerKey }), [ownerKey]);
  const currentLifetime = useRef(lifetime);
  currentLifetime.current = lifetime;
  const [mounted, setMounted] = useState<{ owner: typeof lifetime; service: ExternalUIConnectionService } | null>(null);
  const [destination, setDestination] = useState<{ owner: typeof lifetime; target: ExternalUIDestination | null } | null>(null);

  useEffect(() => {
    if (!ownerKey) return;
    const service = createService();
    let disposed = false;
    let pending = false;
    setMounted({ owner: lifetime, service });
    // Read-only no primeiro mount; nunca cria um convite automaticamente.
    void service.refresh().catch(() => undefined);
    const heartbeat = async () => {
      if (disposed || pending) return;
      const state = service.getSnapshot()?.state;
      if (state !== 'connected' && state !== 'waiting_claim') return;
      pending = true;
      try {
        if (state === 'connected') await service.heartbeat();
        else await service.refresh();
      } catch {
        // Releitura autoritativa observa expiração/revogação. Sem retry de ação.
        if (!disposed) await service.refresh().catch(() => undefined);
      } finally { pending = false; }
    };
    const timer = window.setInterval(() => { void heartbeat(); }, 10_000);
    return () => {
      disposed = true;
      window.clearInterval(timer);
      service.dispose();
    };
  }, [ownerKey, lifetime, createService]);

  const setTarget = useCallback((target: ExternalUIDestination | null) => {
    if (ownerKey && currentLifetime.current === lifetime) setDestination({ owner: lifetime, target });
  }, [ownerKey, lifetime]);
  const value = useMemo<ExternalUIConnectionContextValue>(() => ({
    service: mounted?.owner === lifetime ? mounted.service : null,
    target: destination?.owner === lifetime ? destination.target : null,
    setTarget,
  }), [mounted, destination, lifetime, setTarget]);
  return <ExternalUIConnectionContext.Provider value={value}>{children}</ExternalUIConnectionContext.Provider>;
}
