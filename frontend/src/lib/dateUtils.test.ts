import { describe, expect, it, vi } from 'vitest';
import { formatRelativeTime, formatRelativeTimeLocalized, formatDate, formatDateTime } from './dateUtils';

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

  it('formata tempo relativo conforme o idioma (Intl)', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2024-01-01T12:00:00.000Z'));
    const fiveMinutesAgo = new Date('2024-01-01T11:55:00.000Z').getTime();

    const pt = formatRelativeTimeLocalized(fiveMinutesAgo, 'pt-BR');
    const en = formatRelativeTimeLocalized(fiveMinutesAgo, 'en');
    const es = formatRelativeTimeLocalized(fiveMinutesAgo, 'es');

    // Cada idioma usa seu próprio formato; en/es não devem conter texto pt-BR fixo.
    expect(en.toLowerCase()).toContain('ago');
    expect(en.toLowerCase()).not.toContain('há');
    expect(es.toLowerCase()).toContain('hace');
    expect(pt.toLowerCase()).toContain('há');
    expect(pt).not.toBe(en);

    vi.useRealTimers();
  });

  it('mantém a saída estável em chamadas repetidas (cache por locale)', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2024-01-01T12:00:00.000Z'));
    const fiveMinutesAgo = new Date('2024-01-01T11:55:00.000Z').getTime();

    // Chamar duas vezes com o mesmo locale (formatter reusado do cache de módulo)
    // produz exatamente a mesma saída — comportamento intacto.
    const first = formatRelativeTimeLocalized(fiveMinutesAgo, 'pt-BR');
    const second = formatRelativeTimeLocalized(fiveMinutesAgo, 'pt-BR');
    expect(second).toBe(first);

    vi.useRealTimers();
  });

  it('trunca corretamente diferenças negativas (abs antes do floor)', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2024-01-01T12:00:00.000Z'));
    // 1.1s no passado: deve dar 1 segundo (não 2 — floor de -1.1 daria -2).
    const oneTenthPast = Date.now() - 1100;

    // Asserções tolerantes a variações de ICU/Node (ex.: "1 second ago" vs
    // "1 sec. ago"): valida idioma correto + magnitude 1 (não 2), sem exigir a
    // string exata.
    const en = formatRelativeTimeLocalized(oneTenthPast, 'en');
    expect(en).toMatch(/1\s*sec(ond)?s?\.?\s*ago/i);
    expect(en.toLowerCase()).toContain('ago');
    expect(en).not.toMatch(/\b2\b/);

    const pt = formatRelativeTimeLocalized(oneTenthPast, 'pt-BR');
    expect(pt).toMatch(/1\s*seg(undo)?s?/i);
    expect(pt.toLowerCase()).toContain('há');
    expect(pt).not.toMatch(/\b2\b/);

    vi.useRealTimers();
  });

  it('formata data e data/hora', () => {
    const date = new Date('2024-02-03T10:05:00.000Z');
    expect(formatDate(date)).toMatch(/\d{2}\/\d{2}\/\d{4}/);
    expect(formatDateTime(date)).toMatch(/\d{2}\/\d{2}\/\d{4}/);
  });
});
