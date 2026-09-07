import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ToastHost } from './ToastHost';
import { useUIStore } from '../../store/uiStore';

describe('ToastHost', () => {
  afterEach(() => {
    useUIStore.setState({ toasts: [] });
    vi.restoreAllMocks();
  });

  it('renderiza a lista de toasts da uiStore', () => {
    useUIStore.setState({
      toasts: [
        { id: 't1', message: 'Primeiro toast', type: 'info', duration: 0 },
        { id: 't2', message: 'Segundo toast', type: 'success', duration: 0 },
      ],
    });

    render(<ToastHost />);

    expect(screen.getByText('Primeiro toast')).toBeInTheDocument();
    expect(screen.getByText('Segundo toast')).toBeInTheDocument();
  });

  it('chama removeToast com o id correto ao fechar um toast', () => {
    useUIStore.setState({
      toasts: [{ id: 'abc-123', message: 'Fechar este', type: 'info', duration: 0 }],
    });
    const removeSpy = vi.spyOn(useUIStore.getState(), 'removeToast');

    render(<ToastHost />);

    fireEvent.click(screen.getByRole('button', { name: 'ui.toast.close' }));

    expect(removeSpy).toHaveBeenCalledWith('abc-123');
    expect(useUIStore.getState().toasts.find((t) => t.id === 'abc-123')).toBeUndefined();
  });

  it('nao renderiza nada quando nao ha toasts', () => {
    useUIStore.setState({ toasts: [] });

    const { container } = render(<ToastHost />);

    expect(container.firstChild).toBeNull();
  });
});
