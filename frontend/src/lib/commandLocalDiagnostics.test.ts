import { afterEach, expect, it, vi } from 'vitest';
import { beginLocalPaletteDiagnostic } from './commandLocalDiagnostics';

afterEach(() => { Reflect.deleteProperty(window, 'go'); vi.restoreAllMocks(); });

it('registra apenas identidade e resultado sem esperar a bridge', () => {
  const send = vi.fn(() => new Promise<void>(() => {}));
  Object.defineProperty(window, 'go', { configurable: true, value: { app: { App: { ReportLocalPaletteCommand: send } } } });
  const finish = beginLocalPaletteDiagnostic('navigation.settings.open');
  expect(finish('succeeded')).toBeUndefined();
  expect(send).toHaveBeenCalledOnce();
  expect(send).toHaveBeenCalledWith('navigation.settings.open', expect.any(String), 'succeeded', expect.any(Number), 'local_ui', 'palette');
});

it('não interfere quando a bridge falha ou não existe', () => {
  expect(() => beginLocalPaletteDiagnostic('navigation.settings.open')('cancelled')).not.toThrow();
  Object.defineProperty(window, 'go', { configurable: true, value: { app: { App: { ReportLocalPaletteCommand: () => { throw new Error('offline'); } } } } });
  expect(() => beginLocalPaletteDiagnostic('navigation.settings.open')('failed')).not.toThrow();
});
