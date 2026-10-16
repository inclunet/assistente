import { useCallback, useEffect, useRef, useState } from 'react';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { GetProfiles } from '@wailsjs/go/wailsapi/Profiles';
import type { profiles } from '../../wailsjs/go/models';

export interface CommandProfilesState {
  profiles: profiles.ProfileInfo[];
  loading: boolean;
  error: boolean;
  reload: () => Promise<void>;
}

interface ProfilesSnapshot {
  identityKey: string;
  profiles: profiles.ProfileInfo[];
  loading: boolean;
  error: boolean;
}

/** Carrega o catálogo oficial sem deixar respostas de outra identidade vazarem. */
export function useCommandProfiles(identityKey: string): CommandProfilesState {
  const identityRef = useRef(identityKey);
  identityRef.current = identityKey;
  const [snapshot, setSnapshot] = useState<ProfilesSnapshot>({
    identityKey,
    profiles: [],
    loading: true,
    error: false,
  });
  const requestId = useRef(0);

  const reload = useCallback(async () => {
    const requestIdentity = identityRef.current;
    const request = ++requestId.current;
    setSnapshot((current) => current.identityKey === requestIdentity
      ? { ...current, loading: true, error: false }
      : current);
    try {
      const next = await GetProfiles();
      if (request !== requestId.current || identityRef.current !== requestIdentity) return;
      setSnapshot({ identityKey: requestIdentity, profiles: Array.isArray(next) ? next : [], loading: false, error: false });
    } catch {
      if (request !== requestId.current || identityRef.current !== requestIdentity) return;
      setSnapshot({ identityKey: requestIdentity, profiles: [], loading: false, error: true });
    } finally {
      if (request === requestId.current && identityRef.current === requestIdentity) {
        setSnapshot((current) => current.identityKey === requestIdentity ? { ...current, loading: false } : current);
      }
    }
  }, []);

  useEffect(() => {
    requestId.current += 1;
    setSnapshot({ identityKey, profiles: [], loading: true, error: false });
    void reload();

    const unsubscribers = [
      EventsOn('profile:created', reload),
      EventsOn('profile:deleted', reload),
      EventsOn('profile:updated', reload),
    ];
    return () => {
      requestId.current += 1;
      unsubscribers.forEach((unsubscribe) => unsubscribe());
    };
  }, [identityKey, reload]);

  const currentSnapshot = snapshot.identityKey === identityKey ? snapshot : null;
  return {
    profiles: currentSnapshot?.profiles ?? [],
    loading: currentSnapshot?.loading ?? true,
    error: currentSnapshot?.error ?? false,
    reload,
  };
}
