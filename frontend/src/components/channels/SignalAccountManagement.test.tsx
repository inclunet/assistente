import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { SignalAccountManagement } from './SignalAccountManagement';

describe('SignalAccountManagement', () => {
  const mockOnUnregister = vi.fn();

  it('não renderiza nada quando não há contas', () => {
    const { container } = render(
      <SignalAccountManagement
        accounts={[]}
        unregistering={null}
        onUnregister={mockOnUnregister}
      />
    );

    expect(container.firstChild).toBeNull();
  });

  it('mostra conta conectada quando há uma conta', () => {
    render(
      <SignalAccountManagement
        accounts={['+5511999999999']}
        unregistering={null}
        onUnregister={mockOnUnregister}
      />
    );

    expect(screen.getByText('Conta Conectada')).toBeInTheDocument();
    expect(screen.getByText('+5511999999999')).toBeInTheDocument();
    expect(screen.getByText('Desconectar')).toBeInTheDocument();
  });

  it('chama onUnregister ao clicar em Desconectar', async () => {
    const user = userEvent.setup();
    render(
      <SignalAccountManagement
        accounts={['+5511999999999']}
        unregistering={null}
        onUnregister={mockOnUnregister}
      />
    );

    await user.click(screen.getByText('Desconectar'));
    expect(mockOnUnregister).toHaveBeenCalledWith('+5511999999999');
  });

  it('mostra loading quando está desconectando a conta', () => {
    const { container } = render(
      <SignalAccountManagement
        accounts={['+5511999999999']}
        unregistering="+5511999999999"
        onUnregister={mockOnUnregister}
      />
    );

    const button = container.querySelector('button');
    expect(button).toBeDisabled();
  });

  it('desabilita botão quando qualquer conta está sendo desconectada', () => {
    render(
      <SignalAccountManagement
        accounts={['+5511999999999']}
        unregistering="+5511888888888"
        onUnregister={mockOnUnregister}
      />
    );

    const button = screen.getByText('Desconectar');
    expect(button).toBeDisabled();
  });
});
