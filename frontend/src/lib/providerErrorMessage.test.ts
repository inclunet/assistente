import { describe, expect, it } from 'vitest';
import i18n from './i18n';
import { translateProviderErrorMessage } from './providerErrorMessage';

describe('translateProviderErrorMessage', () => {
  it('localiza marcadores permitidos e mantém o alias informado pelo provedor', async () => {
    await i18n.changeLanguage('pt-BR');

    expect(translateProviderErrorMessage('provider_error:v1:unsupported_parameter:max_completion_tokens:400'))
      .toBe('O provedor não aceita o parâmetro max_completion_tokens para este modelo. Confira os parâmetros configurados. (HTTP 400)');
  });

  it('descarta campos livres e marcadores malformados', async () => {
    await i18n.changeLanguage('pt-BR');

    expect(translateProviderErrorMessage('provider_error:v1:unsupported_parameter:customer_secret'))
      .toBe('O provedor recusou um parâmetro, mas não identificou um campo que possa ser omitido com segurança.');
    expect(translateProviderErrorMessage('provider_error:v1:invalid_value:'))
      .toBe('O provedor recusou um valor da configuração. Confira os parâmetros do modelo.');
    expect(translateProviderErrorMessage('provider_error:v1:unknown:temperature')).toBeNull();
    expect(translateProviderErrorMessage('provider_error:v1:invalid_value:temperature:999')).toBeNull();
  });
});
