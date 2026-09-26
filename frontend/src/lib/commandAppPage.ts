export const APP_PAGES = [
  'workspace', 'settings', 'profiles', 'history', 'help', 'about', 'update', 'tasklists', 'jobs', 'memories',
] as const;

export type AppPage = typeof APP_PAGES[number];

export function isAppPage(value: unknown): value is AppPage {
  return typeof value === 'string' && (APP_PAGES as readonly string[]).includes(value);
}

/** Maps only actual router paths. Settings subroutes and tabs remain one page. */
export function resolveAppPage(pathname: unknown): AppPage | null {
  if (typeof pathname !== 'string' || !pathname.startsWith('/') || pathname.includes('?') || pathname.includes('#')) return null;
  const path = pathname.length > 1 ? pathname.replace(/\/+$/, '') : pathname;
  if (path === '/') return 'workspace';
  if (/^\/settings(?:\/[^/]+)?$/.test(path)) return 'settings';
  switch (path) {
    case '/profiles': return 'profiles';
    case '/history': return 'history';
    case '/help': return 'help';
    case '/about': return 'about';
    case '/update': return 'update';
    case '/tasklists': return 'tasklists';
    case '/jobs': return 'jobs';
    case '/memories': return 'memories';
    default: return null;
  }
}
