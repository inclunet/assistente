import { describe, expect, it } from 'vitest';
import { APP_PAGES, isAppPage, resolveAppPage } from './commandAppPage';

describe('trusted application-page enum', () => {
  it('maps actual router pages and groups every settings subroute as settings', () => {
    expect(resolveAppPage('/')).toBe('workspace');
    for (const path of ['/settings', '/settings/providers', '/settings/commands', '/settings/restore-defaults/']) {
      expect(resolveAppPage(path)).toBe('settings');
    }
    expect(Object.fromEntries([
      ['/profiles', 'profiles'], ['/history', 'history'], ['/help', 'help'], ['/about', 'about'],
      ['/update', 'update'], ['/tasklists', 'tasklists'], ['/jobs', 'jobs'], ['/memories', 'memories'],
    ].map(([path]) => [path, resolveAppPage(path)]))).toEqual({
      '/profiles': 'profiles', '/history': 'history', '/help': 'help', '/about': 'about',
      '/update': 'update', '/tasklists': 'tasklists', '/jobs': 'jobs', '/memories': 'memories',
    });
    expect(APP_PAGES).toEqual(['workspace', 'settings', 'profiles', 'history', 'help', 'about', 'update', 'tasklists', 'jobs', 'memories']);
  });

  it('fails closed for absent, unknown and malformed routes or page facts', () => {
    for (const path of ['', '/settings-extra', '/settings/commands/extra', '/unknown', '/profiles?tab=all', null, {}]) {
      expect(resolveAppPage(path)).toBeNull();
    }
    for (const invalidPage of [undefined, null, '', 'chat', 'tasklist', 'Settings', 'settings.commands', 'unknown']) {
      expect(isAppPage(invalidPage)).toBe(false);
    }
  });
});
