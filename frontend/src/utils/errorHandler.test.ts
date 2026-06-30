import { describe, expect, it, vi } from 'vitest';
import { handleError, withErrorHandling, retryWithBackoff, ErrorSeverity } from './errorHandler';

const announceSpy = vi.fn();

vi.mock('../hooks/useAnnouncer', () => ({
  announce: (...args: unknown[]) => announceSpy(...args),
}));

describe('errorHandler', () => {
  it('anuncia erro', () => {
    handleError(new Error('x'), {
      source: 'test',
      userMessage: 'Falha',
      severity: ErrorSeverity.RECOVERABLE,
    });

    expect(announceSpy).toHaveBeenCalledWith('Falha', 'polite');
  });

  it('respeita override de prioridade assertive', () => {
    handleError(new Error('x'), {
      source: 'test',
      userMessage: 'Falha crítica',
      severity: ErrorSeverity.RECOVERABLE,
      announcePriority: 'assertive',
    });

    expect(announceSpy).toHaveBeenCalledWith('Falha crítica', 'assertive');
  });

  it('wrap com erro', async () => {
    const fn = vi.fn(async () => {
      throw new Error('boom');
    });

    const wrapped = withErrorHandling(fn, {
      source: 'test',
      userMessage: 'Falha',
      severity: ErrorSeverity.USER,
    });

    await expect(wrapped()).rejects.toThrow('boom');
  });

  it('retryWithBackoff tenta novamente', async () => {
    vi.useFakeTimers();

    let attempts = 0;
    const fn = vi.fn(async () => {
      attempts += 1;
      if (attempts < 2) throw new Error('x');
      return 'ok';
    });

    const promise = retryWithBackoff(fn, { initialDelay: 10, maxRetries: 2 });
    await vi.runAllTimersAsync();

    await expect(promise).resolves.toBe('ok');
    expect(fn).toHaveBeenCalledTimes(2);

    vi.useRealTimers();
  });
});
