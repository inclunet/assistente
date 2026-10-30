import { expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { BasePicker } from './BasePicker';

it('associa ajuda ao botão e ao campo de busca reais', async () => {
 const user = userEvent.setup();
 render(<BasePicker variant="form" items={[{value: 'model', label: 'Modelo'}]} selected="" onSelect={vi.fn()} label="Modelo TTS" helpText="Informe o ID exato do modelo." />);
 const trigger = screen.getByRole('button', { name: /Modelo TTS/ });
 expect(trigger).toHaveAccessibleDescription('Informe o ID exato do modelo.');
 await user.click(trigger);
 expect(screen.getByRole('combobox')).toHaveAccessibleDescription('Informe o ID exato do modelo.');
});
