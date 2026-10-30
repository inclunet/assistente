import { useState } from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { axe } from 'vitest-axe';
import { llmcatalog } from '@wailsjs/go/models';
import { ProviderModelAliases } from './ProviderModelAliases';

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'pt-BR' } }),
}));

const model = llmcatalog.Model.createFrom({ id: 'vendor/model', name: 'Canonical' });
function Editor({ initial = {}, models = [model] }: { initial?: Record<string, string>; models?: llmcatalog.Model[] }) {
  const [aliases, setAliases] = useState(initial);
  return <ProviderModelAliases aliases={aliases} onChange={setAliases} remoteModels={['remote']} catalogModels={models} />;
}

describe('ProviderModelAliases', () => {
  it('vincula um ID manual exato e permite remover o vínculo', async () => {
    const user = userEvent.setup();
    const { container } = render(<Editor />);
    expect(await axe(container)).toHaveNoViolations();
    await user.click(screen.getByRole('button', { name: /providerForm.aliases.remote/ }));
    await user.type(screen.getByRole('combobox', { name: /providerForm.aliases.remote/ }), 'team/private');
    await waitFor(() => expect(screen.getByRole('combobox', { name: /providerForm.aliases.remote/ })).toHaveFocus());
    await user.keyboard('{Enter}');
    const canonical = screen.getByLabelText('providerForm.aliases.canonical');
    await user.selectOptions(canonical, 'vendor/model');
    expect(canonical).toHaveValue('vendor/model');
    await user.click(screen.getByRole('button', { name: /providerForm.aliases.remote/ }));
    expect(await screen.findByRole('option', { name: /team\/private/ })).toBeInTheDocument();
    await user.keyboard('{Escape}');
    await user.selectOptions(canonical, '');
    expect(canonical).toHaveValue('');
  });

  it('mantém o vínculo salvo ausente do catálogo e permite removê-lo offline', async () => {
    const user = userEvent.setup();
    render(<Editor initial={{ remote: 'removed' }} models={[]} />);
    await user.click(screen.getByRole('button', { name: /providerForm.aliases.remote/ }));
    await waitFor(() => expect(screen.getByRole('combobox', { name: /providerForm.aliases.remote/ })).toHaveFocus());
    await user.keyboard('{Enter}');
    const canonical = screen.getByLabelText('providerForm.aliases.canonical');
    await waitFor(() => expect(canonical).toHaveValue('removed'));
    await user.selectOptions(canonical, '');
    expect(canonical).toHaveValue('');
  });
});


it.each(['{Enter}', '{Tab}'])('preserva espaços significativos no ID manual com %s', async keys => {
  const user = userEvent.setup();
  const onChange = vi.fn();
  render(<ProviderModelAliases aliases={{}} remoteModels={[]} catalogModels={[model]} onChange={onChange} />);
  await user.click(screen.getByRole('button', { name: /providerForm.aliases.remote/ }));
  await user.type(screen.getByRole('combobox', { name: /providerForm.aliases.remote/ }), ' team/private ');
  await user.keyboard(keys);
  await user.selectOptions(screen.getByLabelText('providerForm.aliases.canonical'), 'vendor/model');
  expect(onChange).toHaveBeenCalledWith({ ' team/private ': 'vendor/model' });
});
