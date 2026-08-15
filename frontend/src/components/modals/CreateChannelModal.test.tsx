import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import CreateChannelModal from './CreateChannelModal';

const getTemplatesSpy = vi.fn();
const createFromTemplateSpy = vi.fn();

vi.mock('react-i18next', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-i18next')>();
  return {
    ...actual,
    useTranslation: () => ({ t: (key: string) => key }),
  };
});

vi.mock('@wailsjs/go/wailsapi/Messaging', () => ({
  GetChannelTemplates: () => getTemplatesSpy(),
  CreateChannelFromTemplate: (...args: unknown[]) => createFromTemplateSpy(...args),
}));

describe('CreateChannelModal', () => {
  it('carrega templates, seleciona e cria canal', async () => {
    getTemplatesSpy.mockResolvedValueOnce([
      {
        type: 'signal',
        display_name: 'Signal',
        description: 'Desc',
        icon: 'S',
        fields: [
          { key: 'api', label: 'API', type: 'text', required: true, placeholder: 'x' },
        ],
      },
    ]);
    createFromTemplateSpy.mockResolvedValueOnce(undefined);

    const onClose = vi.fn();
    const onSuccess = vi.fn();

    render(
      <CreateChannelModal
        isOpen={true}
        onClose={onClose}
        onSuccess={onSuccess}
      />
    );

    await screen.findByText('Signal');
    fireEvent.click(screen.getByRole('button', { name: /Signal/i }));

    const input = screen.getByLabelText(/API/);
    fireEvent.change(input, { target: { value: 'token' } });

    fireEvent.click(screen.getByRole('button', { name: 'channels.createModal.create' }));

    await waitFor(() => {
      expect(createFromTemplateSpy).toHaveBeenCalledWith('signal', { api: 'token' });
      expect(onSuccess).toHaveBeenCalled();
      expect(onClose).toHaveBeenCalled();
    });
  });
});
