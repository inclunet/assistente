import { describe, expect, it } from 'vitest';
import i18n from './i18n';
import { translateBackendErrorDiagnostic, translateErrorMarkers, translateProviderErrorMessage } from './providerErrorMessage';

describe('translateProviderErrorMessage', () => {
  it('traduz violações do contrato com seus limites nos três idiomas', () => {
    const marker = 'parameter_error:v1:max_output_tokens:maximum:128000';
    expect(translateProviderErrorMessage(marker, 'pt-BR')).toBe('O valor de max_output_tokens deve ser no máximo 128000.');
    expect(translateProviderErrorMessage(marker, 'en')).toBe('The value for max_output_tokens must be at most 128000.');
    expect(translateProviderErrorMessage(marker, 'es')).toBe('El valor de max_output_tokens debe ser como máximo 128000.');
    expect(translateProviderErrorMessage('parameter_error:v1:temperature:minimum')).toBeNull();
    expect(translateProviderErrorMessage('parameter_error:v1:unknown:maximum:2')).toBeNull();
    expect(translateProviderErrorMessage('parameter_error:v1:max_output_tokens:required', 'en')).toBe('Configure max_output_tokens: this API requires an explicit value and no model default is available.');
  });

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
  it('traduz erros de parâmetros persistidos sem reescrever texto livre', () => {
    const diagnostic = 'Falha na resposta do agente: parameter_error:v1:max_output_tokens:required';
    expect(translateBackendErrorDiagnostic(diagnostic, 'pt-BR')).toBe('Falha na resposta do agente: Configure max_output_tokens: esta API exige um valor explícito e não há default disponível para o modelo.');
    expect(translateBackendErrorDiagnostic(diagnostic, 'en')).toBe('Agent response failed: Configure max_output_tokens: this API requires an explicit value and no model default is available.');
    expect(translateBackendErrorDiagnostic(diagnostic, 'es')).toBe('Falló la respuesta del agente: Configura max_output_tokens: esta API exige un valor explícito y no hay un valor predeterminado disponible para el modelo.');
    const prose = `${diagnostic} is an example`;
    expect(translateBackendErrorDiagnostic(prose, 'en')).toBe(prose);
  });

  it('traduz marcadores dentro de erros persistidos com prefixo', async () => {
    await i18n.changeLanguage('pt-BR');
    expect(translateErrorMarkers('Falha na resposta do agente: provider_error:v1:authentication:'))
      .toBe('Falha na resposta do agente: O provedor recusou a autenticação. Confira a credencial configurada.');
    await i18n.changeLanguage('es');
    expect(translateBackendErrorDiagnostic('Falha na resposta do agente: provider_error:v1:authentication:'))
      .toBe('Falló la respuesta del agente: El proveedor rechazó la autenticación. Comprueba la credencial configurada.');
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
    expect(translateBackendErrorDiagnostic('Falha na resposta do agente: mcp_error:v1:Servidor%20A:handshake'))
      .toBe('El servidor MCP Servidor A falló durante la conexión. Inténtalo de nuevo o comprueba sus credenciales y conexión.');
  });

  it('usa o idioma do perfil para diagnósticos MCP sem alterar o idioma da interface', async () => {
    await i18n.changeLanguage('pt-BR');
    expect(translateBackendErrorDiagnostic('Falha na resposta do agente: mcp_error:v1:Servidor%20A:call', 'es-ES'))
      .toBe('El servidor MCP Servidor A falló al ejecutar una herramienta. Inténtalo de nuevo o comprueba sus credenciales y conexión.');
    expect(i18n.language).toBe('pt-BR');
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
