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
});
