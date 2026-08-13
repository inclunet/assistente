import { describe, expect, it } from 'vitest';
import type { TFunction } from 'i18next';
import { formatPortabilityMessage, portabilityMessageKey } from './portabilityMessages';

// Mock de TFunction: ecoa a chave e os parâmetros pedidos, para o teste ver o
// que a UI mandou traduzir.
const echoT = ((key: string, opts?: Record<string, unknown>) => (
  opts ? `${key}|${JSON.stringify(opts.replace ?? {})}` : key
)) as unknown as TFunction;

// Mock que finge não conhecer a chave, como um app cuja tradução ainda não tem
// o código: o i18next devolve o defaultValue.
const missingKeyT = ((_key: string, opts?: { defaultValue?: string }) => (
  opts?.defaultValue ?? ''
)) as unknown as TFunction;

describe('formatPortabilityMessage', () => {
  it('traduz o código com o prefixo de portabilidade e passa os parâmetros', () => {
    const text = formatPortabilityMessage(
      {
        code: 'acp.commandNotFound',
        params: { providerId: 'cursor', command: 'cursor-agent' },
        message: 'Provider "cursor" usa o agente "cursor-agent".',
      },
      echoT,
    );

    expect(text).toBe(
      'portability.messages.acp.commandNotFound|{"providerId":"cursor","command":"cursor-agent"}',
    );
  });

  it('cai no texto que o backend mandou quando a tradução não conhece o código', () => {
    const text = formatPortabilityMessage(
      { code: 'algo.queAindaNaoExiste', message: 'Texto de reserva do backend.' },
      missingKeyT,
    );

    expect(text).toBe('Texto de reserva do backend.');
  });

  it('usa o texto de reserva quando não vem código', () => {
    const text = formatPortabilityMessage({ message: 'erro ao criar conversa: disco cheio' }, echoT);

    expect(text).toBe('erro ao criar conversa: disco cheio');
  });

  // Os parâmetros vêm de arquivo importado, mas o código não: recusar o que não
  // parece identificador impede que um arquivo alcance uma chave qualquer.
  it('ignora código fora do formato de identificador', () => {
    const text = formatPortabilityMessage(
      { code: 'common.cancel Bem tentado', message: 'Texto de reserva.' },
      echoT,
    );

    expect(text).toBe('Texto de reserva.');
  });

  it('aceita a lista antiga, de texto puro', () => {
    expect(formatPortabilityMessage('aviso antigo', echoT)).toBe('aviso antigo');
  });
});

describe('portabilityMessageKey', () => {
  it('distingue mensagens de mesmo código pela posição', () => {
    const first = portabilityMessageKey({ code: 'acp.credentialMissing', message: 'a' }, 0);
    const second = portabilityMessageKey({ code: 'acp.credentialMissing', message: 'a' }, 1);

    expect(first).not.toBe(second);
  });
});
