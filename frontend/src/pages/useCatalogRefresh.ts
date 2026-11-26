import { useEffect, useState } from 'react';
import type { plugins } from '@wailsjs/go/models';
import { RefreshRepository } from '@wailsjs/go/wailsapi/Plugins';

export const catalogRefreshInterval = 15 * 60_000;

// The catalog and the repository list can mount in succession during one read.
const attempts = new Map<string, { time: number; failed: boolean; running?: Promise<boolean> }>();

function update(session: string, repo: plugins.Repository): Promise<boolean> | undefined {
  const now = Date.now();
  const key = JSON.stringify([session, repo.id, repo.revision]);
  const attempt = attempts.get(key);
  if (attempt?.running) return attempt.running;
  if (now - new Date(repo.updated).getTime() < catalogRefreshInterval) return;
  if (attempt && now - attempt.time < catalogRefreshInterval) return;
  const next = { time: now, failed: false, running: undefined as Promise<boolean> | undefined };
  attempts.set(key, next);
  next.running = RefreshRepository(repo.id, repo.revision).then(() => true, () => {
    next.failed = true;
    return false;
  }).finally(() => { next.running = undefined; });
  return next.running;
}

export function useCatalogRefresh(session: string, rows: plugins.Repository[], paused: boolean, reload: () => Promise<void>) {
  const [offline, setOffline] = useState(false);
  useEffect(() => {
    if (paused) return;
    let active = true;
    let running = false;
    async function check() {
      if (!active || running || document.hidden) return;
      running = true;
      let changed = false;
      let failed = false;
      try {
        for (const [key, attempt] of attempts) {
          if (!attempt.running && Date.now() - attempt.time >= catalogRefreshInterval) attempts.delete(key);
        }
        for (const repo of rows) {
          if (!active || document.hidden) return;
          const result = update(session, repo);
          if (result) {
            if (await result) changed = true;
            else failed = true;
          } else {
            const key = JSON.stringify([session,repo.id,repo.revision]);
            if (attempts.get(key)?.failed && Date.now() - new Date(repo.updated).getTime() >= catalogRefreshInterval) failed = true;
          }
        }
        if (changed && active) await reload();
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
