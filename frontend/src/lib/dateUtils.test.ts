import { describe, expect, it, vi } from 'vitest';
import { formatRelativeTime, formatDate, formatDateTime } from './dateUtils';

describe('dateUtils', () => {
  it('formata tempo relativo', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2024-01-01T12:00:10.000Z'));

    const base = new Date('2024-01-01T12:00:05.000Z').getTime();
    expect(formatRelativeTime(base)).toBe('agora');

    const fiveMinutes = new Date('2024-01-01T11:55:00.000Z').getTime();
    expect(formatRelativeTime(fiveMinutes)).toBe('há 5 min');

    vi.useRealTimers();
  });

  it('formata data e data/hora', () => {
    const date = new Date('2024-02-03T10:05:00.000Z');
    expect(formatDate(date)).toMatch(/\d{2}\/\d{2}\/\d{4}/);
    expect(formatDateTime(date)).toMatch(/\d{2}\/\d{2}\/\d{4}/);
  });
});
