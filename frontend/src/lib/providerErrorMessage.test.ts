import { describe, expect, it } from 'vitest';
import i18n from './i18n';
import { translateBackendErrorDiagnostic, translateErrorMarkers, translateProviderErrorMessage } from './providerErrorMessage';

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
    expect(translateProviderErrorMessage('provider_error:v1:invalid_value:temperature:400'))
      .toContain('temperature');
    expect(translateProviderErrorMessage('provider_error:v1:unknown:temperature')).toBeNull();
    expect(translateProviderErrorMessage('provider_error:v1:invalid_value:temperature:999')).toBeNull();
  });
});

describe('translateErrorMarkers', () => {
  it('traduz marcadores dentro de erros persistidos com prefixo', async () => {
    await i18n.changeLanguage('pt-BR');
    expect(translateErrorMarkers('Falha na resposta do agente: provider_error:v1:authentication:'))
      .toBe('Falha na resposta do agente: O provedor recusou a autenticação. Confira a credencial configurada.');
  });

  it('traduz falhas MCP por servidor e etapa em cada idioma', async () => {
    await i18n.changeLanguage('pt-BR');
    expect(translateErrorMarkers('mcp_error:v1:Servidor%20A:list_tools'))
      .toBe('O servidor MCP Servidor A falhou ao listar ferramentas. Tente novamente ou verifique as credenciais e a conexão.');
    await i18n.changeLanguage('en');
    expect(translateErrorMarkers('mcp_error:v1:Servidor%20A:call'))
      .toBe('MCP server Servidor A failed while running a tool. Try again or check its credentials and connection.');
    await i18n.changeLanguage('es');
    expect(translateErrorMarkers('mcp_error:v1:Servidor%20A:handshake'))
      .toBe('El servidor MCP Servidor A falló durante la conexión. Inténtalo de nuevo o comprueba sus credenciales y conexión.');
  });

  it('decodifica dois-pontos escapados e preserva marcadores literais', async () => {
    await i18n.changeLanguage('pt-BR');
    expect(translateErrorMarkers('mcp_error:v1:Equipe%3A%20Produ%C3%A7%C3%A3o:call'))
      .toContain('Equipe: Produção');
    const code = 'const diagnostic = "provider_error:v1:authentication:";';
    expect(translateBackendErrorDiagnostic(code)).toBe(code);
    expect(translateBackendErrorDiagnostic('Falha na resposta do agente: provider_error:v1:authentication:'))
      .toContain('O provedor recusou a autenticação.');
  });
});
