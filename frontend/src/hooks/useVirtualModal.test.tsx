import { useRef } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { render, screen, act } from '@testing-library/react';
import { useVirtualModal } from './useVirtualModal';

vi.mock('./useAnnouncer', () => ({
  announce: () => {},
}));

interface HarnessProps {
  isActive: boolean;
  onClose?: () => void;
}

function VirtualModalHarness({ isActive, onClose = () => {} }: HarnessProps) {
  const ref = useRef<HTMLDivElement>(null);
  useVirtualModal({ elementRef: ref, isActive, onClose });
  return (
    <div ref={ref} data-testid="message-node">
      <div className="chat-message__text" data-testid="message-text">
        Conteudo da mensagem
      </div>
    </div>
  );
}

// Estrutura de um turno agêntico: a cadeia inteira (vários segmentos de texto +
// blocos de tool calls) vive dentro de `.chat-message__content`.
function AgenticHarness({ isActive, onClose = () => {} }: HarnessProps) {
  const ref = useRef<HTMLDivElement>(null);
  useVirtualModal({ elementRef: ref, isActive, onClose });
  return (
    <div ref={ref} data-testid="message-node">
      <div className="chat-message" aria-label="conclusao do turno">
        <div className="chat-message__content" data-testid="content">
          <div className="chat-message__text chat-message__text--segment" data-testid="seg-1">
            texto intro
          </div>
          <div className="tool-calls-section" data-testid="tool-1">ferramenta A</div>
          <div className="chat-message__text chat-message__text--segment" data-testid="seg-2">
            texto meio
          </div>
          <div className="chat-message__text chat-message__text--segment" data-testid="seg-3">
            resposta final
          </div>
        </div>
      </div>
    </div>
  );
}

describe('useVirtualModal', () => {
  it('marca o elemento ancora como role="dialog" e o conteudo como role="document" quando ativo', () => {
    render(<VirtualModalHarness isActive={true} />);

    const dialog = screen.getByTestId('message-node');
    const content = screen.getByTestId('message-text');

    expect(dialog).toHaveAttribute('role', 'dialog');
    expect(dialog).toHaveAttribute('aria-modal', 'true');
    expect(content).toHaveAttribute('role', 'document');
    expect(content).toHaveAttribute('tabindex', '0');
  });

  it('restaura os atributos do conteudo ao sair do modo leitura', () => {
    const { rerender } = render(<VirtualModalHarness isActive={true} />);
    const content = screen.getByTestId('message-text');
    expect(content).toHaveAttribute('role', 'document');

    act(() => {
      rerender(<VirtualModalHarness isActive={false} />);
    });

    expect(content).not.toHaveAttribute('role');
    expect(content).not.toHaveAttribute('tabindex');
  });

  // Issue #163 (Parte A): num turno agêntico o alvo de foco/role="document" deve
  // englobar a CADEIA INTEIRA (`.chat-message__content`) — não o primeiro
  // `.chat-message__text` — para que o leitor de tela navegue por todos os
  // segmentos de texto E as tool calls em ordem.
  it('aplica role="document" no container da cadeia inteira, não no primeiro segmento de texto', () => {
    render(<AgenticHarness isActive={true} />);

    const content = screen.getByTestId('content');
    const firstSegment = screen.getByTestId('seg-1');

    expect(content).toHaveAttribute('role', 'document');
    expect(content).toHaveAttribute('tabindex', '0');
    // O primeiro segmento NÃO é o alvo — antes o role="document" caía nele,
    // restringindo a leitura a um único trecho.
    expect(firstSegment).not.toHaveAttribute('role', 'document');

    // A região de leitura contém todos os segmentos e as tool calls.
    expect(content).toContainElement(screen.getByTestId('seg-2'));
    expect(content).toContainElement(screen.getByTestId('seg-3'));
    expect(content).toContainElement(screen.getByTestId('tool-1'));
  });

  it('restaura os atributos do container da cadeia ao sair do modo leitura', () => {
    const { rerender } = render(<AgenticHarness isActive={true} />);
    const content = screen.getByTestId('content');
    expect(content).toHaveAttribute('role', 'document');

    act(() => {
      rerender(<AgenticHarness isActive={false} />);
    });

    expect(content).not.toHaveAttribute('role');
    expect(content).not.toHaveAttribute('tabindex');
  });
});
