import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { logger } from './logger';

describe('logger', () => {
  const originalLevel = logger.getLevel();

  beforeEach(() => {
    vi.spyOn(console, 'debug').mockImplementation(() => {});
    vi.spyOn(console, 'info').mockImplementation(() => {});
    vi.spyOn(console, 'warn').mockImplementation(() => {});
    vi.spyOn(console, 'error').mockImplementation(() => {});
  });

  afterEach(() => {
    logger.setLevel(originalLevel);
    vi.restoreAllMocks();
  });

  it('emite warn e error mesmo em nível "warn" (produção)', () => {
    logger.setLevel('warn');
    logger.error('boom');
    logger.warn('cuidado');
    logger.info('info');
    logger.debug('debug');
    logger.log('log');

    expect(console.error).toHaveBeenCalledWith('boom');
    expect(console.warn).toHaveBeenCalledWith('cuidado');
    expect(console.info).not.toHaveBeenCalled();
    expect(console.debug).not.toHaveBeenCalled();
  });

  it('emite todos os níveis em "debug" (desenvolvimento)', () => {
    logger.setLevel('debug');
    logger.debug('d');
    logger.log('l');
    logger.info('i');
    logger.warn('w');
    logger.error('e');

    expect(console.debug).toHaveBeenCalledTimes(2); // debug + log
    expect(console.info).toHaveBeenCalledTimes(1);
    expect(console.warn).toHaveBeenCalledTimes(1);
    expect(console.error).toHaveBeenCalledTimes(1);
  });

  it('silencia tudo em "silent"', () => {
    logger.setLevel('silent');
    logger.error('e');
    logger.warn('w');
    logger.info('i');
    logger.debug('d');

    expect(console.error).not.toHaveBeenCalled();
    expect(console.warn).not.toHaveBeenCalled();
    expect(console.info).not.toHaveBeenCalled();
    expect(console.debug).not.toHaveBeenCalled();
  });

  it('log() encaminha para console.debug', () => {
    logger.setLevel('debug');
    logger.log('mensagem', 42);
    expect(console.debug).toHaveBeenCalledWith('mensagem', 42);
  });
});

describe('logger - nível default por ambiente', () => {
  // O nível default é calculado na carga do módulo a partir de
  // `import.meta.env.DEV` e `VITE_DEBUG`. Para travar essa regra central,
  // stubamos o env e re-importamos o módulo (resetModules) a cada cenário.
  beforeEach(() => {
    vi.spyOn(console, 'debug').mockImplementation(() => {});
    vi.spyOn(console, 'info').mockImplementation(() => {});
    vi.spyOn(console, 'warn').mockImplementation(() => {});
    vi.spyOn(console, 'error').mockImplementation(() => {});
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.resetModules();
    vi.restoreAllMocks();
  });

  const loadFreshLogger = async () => {
    vi.resetModules();
    const mod = await import('./logger');
    return mod.logger;
  };

  it('DEV: nível default é "debug" e emite debug/log', async () => {
    vi.stubEnv('DEV', true);
    vi.stubEnv('VITE_DEBUG', '');
    const log = await loadFreshLogger();

    expect(log.getLevel()).toBe('debug');
    log.debug('d');
    log.log('l');
    expect(console.debug).toHaveBeenCalledTimes(2);
  });

  it('PRODUÇÃO: nível default é "warn" — silencia debug/log/info, mantém warn/error', async () => {
    vi.stubEnv('DEV', false);
    vi.stubEnv('VITE_DEBUG', '');
    const log = await loadFreshLogger();

    expect(log.getLevel()).toBe('warn');
    log.debug('d');
    log.log('l');
    log.info('i');
    log.warn('w');
    log.error('e');

    expect(console.debug).not.toHaveBeenCalled();
    expect(console.info).not.toHaveBeenCalled();
    expect(console.warn).toHaveBeenCalledTimes(1);
    expect(console.error).toHaveBeenCalledTimes(1);
  });

  it('PRODUÇÃO + VITE_DEBUG="true": reabilita debug/log no default', async () => {
    vi.stubEnv('DEV', false);
    vi.stubEnv('VITE_DEBUG', 'true');
    const log = await loadFreshLogger();

    expect(log.getLevel()).toBe('debug');
    log.debug('d');
    log.log('l');
    expect(console.debug).toHaveBeenCalledTimes(2);
  });

  it('PRODUÇÃO + VITE_DEBUG="1": também reabilita debug/log no default', async () => {
    vi.stubEnv('DEV', false);
    vi.stubEnv('VITE_DEBUG', '1');
    const log = await loadFreshLogger();

    expect(log.getLevel()).toBe('debug');
  });
});
