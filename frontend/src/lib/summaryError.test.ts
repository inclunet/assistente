import { describe, it, expect } from 'vitest';
import type { TFunction } from 'i18next';

import { summaryErrorMessage, SUMMARY_ERROR_AGENT_PROVIDER } from './summaryError';

// Mock de TFunction: ecoa a chave + os args interpolados.
const t = ((key: string, opts?: Record<string, unknown>) =>
  opts ? `${key}|${JSON.stringify(opts)}` : key) as unknown as TFunction;

describe('summaryErrorMessage', () => {
  it('traduz o motivo nomeado pelo backend em vez de exibir o texto dele', () => {
    const message = summaryErrorMessage(t, {
      code: SUMMARY_ERROR_AGENT_PROVIDER,
      error: 'Resumo não gerado: o provedor do perfil é um agente externo.',
    });

    expect(message).toBe('app.summary.errors.agentProvider');
  });

  it('cai no texto do evento quando o motivo não tem código', () => {
    expect(summaryErrorMessage(t, { error: 'timeout do provedor' })).toBe(
      'app.summary.error|{"error":"timeout do provedor"}',
    );
  });

  it('cai no texto do evento quando o código é desconhecido', () => {
    expect(summaryErrorMessage(t, { code: 'motivo_novo', error: 'algo' })).toBe(
      'app.summary.error|{"error":"algo"}',
    );
  });
});
