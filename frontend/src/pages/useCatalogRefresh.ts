import { useEffect, useRef, useState } from 'react';
import type { plugins } from '@wailsjs/go/models';
import { RefreshRepository } from '@wailsjs/go/wailsapi/Plugins';

export const catalogRefreshInterval = 15 * 60_000;

// The catalog and the repository list can mount in succession during one read.
interface Attempt {
  time: number;
  result: Promise<boolean>;
  pending: boolean;
}
const attempts = new Map<string, Attempt>();

function update(session: string, repo: plugins.Repository): Attempt | undefined {
  const now = Date.now();
  const key = JSON.stringify([session, repo.id, repo.revision]);
  const previous = attempts.get(key);
  if (previous?.pending) return previous;
  if (now - new Date(repo.updated).getTime() < catalogRefreshInterval) return;
  if (previous && now - previous.time < catalogRefreshInterval) return previous;
  const attempt: Attempt = {
    time: now,
    pending: true,
    result: RefreshRepository(repo.id, repo.revision).then(() => true, () => false)
      .finally(() => { attempt.pending = false; }),
  };
  attempts.set(key, attempt);
  return attempt;
}

export function useCatalogRefresh(session: string, rows: plugins.Repository[], paused: boolean, reload: () => Promise<void>) {
  const published = useRef(new WeakSet<Attempt>());
  const [offline, setOffline] = useState(false);
  useEffect(() => {
    if (paused) return;
    let active = true;
    let running = false;
    async function check() {
      if (!active || running || document.hidden) return;
      running = true;
      const successful: Attempt[] = [];
      let failed = false;
      try {
        for (const [key, attempt] of attempts) {
          if (!attempt.pending && Date.now() - attempt.time >= catalogRefreshInterval) attempts.delete(key);
        }
        for (const repo of rows) {
          if (!active || document.hidden) return;
          const attempt = update(session, repo);
          if (!attempt || published.current.has(attempt)) continue;
          if (await attempt.result) successful.push(attempt);
          else failed = true;
        }
        if (successful.length && active) {
          await reload();
          if (active) successful.forEach(attempt => published.current.add(attempt));
        }
      } catch { failed = true; }
      finally { running = false; if (active) setOffline(failed); }
    }
    void check();
    const interval = window.setInterval(() => void check(), catalogRefreshInterval);
    document.addEventListener('visibilitychange', check);
    window.addEventListener('focus', check);
    return () => {
      active = false;
      window.clearInterval(interval);
      document.removeEventListener('visibilitychange', check);
      window.removeEventListener('focus', check);
    };
  }, [session, rows, paused, reload]);
  return offline;
}
