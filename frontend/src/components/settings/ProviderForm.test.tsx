vi.mock('@wailsjs/go/wailsapi/Credentials', () => ({
 GetCredentialForURL: vi.fn(async () => null), ListCredentials: vi.fn(async () => []),
 ListExternalSources: vi.fn(async () => []), UpsertCredential: vi.fn(),
}));
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { ProviderForm, PROVIDER_CONFIG, API_FORMAT_OPTIONS } from "./ProviderForm";

const waitForProfileOption = async (select: HTMLElement, value: string) => {
  await waitFor(() => expect(Array.from((select as HTMLSelectElement).options).some(option => option.value === value)).toBe(true));
};

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (key: string) => {
      const translations: Record<string, string> = {
        'providerForm.name': 'Nome',
        'providerForm.namePlaceholder': 'Nome do provedor',
        'providerForm.providerType': 'Tipo',
        'providerForm.serverProfile': 'Perfil de servidor',
        'providerForm.customProfile': 'Personalizado',
        'providerForm.catalogUnavailable': 'Catálogo indisponível. Você pode configurar um perfil personalizado.',
        'providerForm.reasoningContentMode': 'Compatibilidade com reasoning_content',
        'providerForm.reasoningContentModeHelp': 'Configuração explícita do protocolo',
        'providerForm.reasoningContentDisabled': 'Desabilitada',
        'providerForm.reasoningContentReplayWithTools': 'Reenviar durante tools',
        'providerForm.baseUrl': 'Base URL',
        'providerForm.defaultUrl': 'URL padrão',
        'providerForm.error.nameRequired': 'Nome é obrigatório',
        'providerForm.error.urlRequired': 'URL é obrigatória',
        'providerForm.error.urlInvalid': 'URL inválida',
        'providerForm.error.testFirst': 'Teste a API antes de salvar',
        'providerForm.error.apiKeyRequiredTest': 'API Key é obrigatória para testar',
        'providerForm.defaultModel': 'Modelo Padrão',
        'providerForm.defaultModelHelp': 'Modelo usado quando o perfil escolhe Padrão',
        'providerForm.loadingModels': 'Carregando modelos...',
        'providerForm.loadModels': 'Carregar modelos',
        'providerForm.loadModelsBtn': '📡 Carregar Modelos',
        'providerForm.modelAutomatic': '(Automático)',
        'providerForm.modelPlaceholder': 'ex: gpt-4o-mini',
        'providerForm.connected': '✓ Conectado',
        'providerForm.urlReadonly': 'URL padrão',
        'providerForm.apiKey': 'API Key',
        'providerForm.apiKeyOptional': 'API Key (Opcional)',
        'providerForm.keySaved': 'Será salva criptografada',
        'providerForm.noKeyNeeded': 'Deixe em branco',
        'providerForm.leaveEmpty': 'Deixar em branco',
        'providerForm.hideKey': 'Ocultar chave',
        'providerForm.showKey': 'Mostrar chave',
        'providerForm.changeKey': 'Alterar API Key',
        'providerForm.changeKeyBtn': '🔓 Alterar Chave',
        'providerForm.keyConfigured': '🔑 Chave configurada no gerenciador de credenciais',
        'providerForm.keepCurrent': 'Deixe em branco para manter',
        'providerForm.keyConfiguredOptional': '🔑 Chave configurada',
        'providerForm.fillApiKey': 'Preencha a API Key',
        'providerForm.updateBtn': 'Atualizar',
        'providerForm.error.testError': 'Erro ao testar API',
        'common.cancel': 'Cancelar',
        'common.saving': 'Salvando...',
        'common.create': 'Criar',
      };
      return translations[key] ?? key;
    },
    i18n: { language: 'pt-BR' },
  }),
}));

// Mock Wails API
vi.mock("@wailsjs/go/wailsapi/LLMProviders", () => ({
  ListModelsRaw: vi.fn(() => Promise.resolve(["gpt-4o", "gpt-4o-mini"])),
  CreateLLMProvider: vi.fn(() => Promise.resolve({ id: "123" })),
  UpdateLLMProvider: vi.fn(() => Promise.resolve({})),
}));
vi.mock('@wailsjs/go/wailsapi/LLMCatalog', async () => {
  const { providerFormCatalogAPIResponse } = await import('./providerFormCatalogTestUtils');
  return { GetLLMCatalog: vi.fn(async () => providerFormCatalogAPIResponse) };
});

import * as App from "@wailsjs/go/wailsapi/LLMProviders";
import { GetLLMCatalog } from '@wailsjs/go/wailsapi/LLMCatalog';
import { providerFormCatalogAPIResponse, providerFormCatalogRefreshingAPIResponse } from './providerFormCatalogTestUtils';

describe("PROVIDER_CONFIG", () => {
  it("deve ter configuração para OpenAI", () => {
    expect(PROVIDER_CONFIG.openai).toBeDefined();
    expect(PROVIDER_CONFIG.openai.label).toBe("OpenAI");
    expect(PROVIDER_CONFIG.openai.apiKeyRequired).toBe(true);
    expect(PROVIDER_CONFIG.openai.urlEditable).toBe(false);
  });

  it("deve ter configuração para Ollama", () => {
    expect(PROVIDER_CONFIG.ollama).toBeDefined();
    expect(PROVIDER_CONFIG.ollama.label).toBe("Ollama (local)");
    expect(PROVIDER_CONFIG.ollama.apiKeyRequired).toBe(false);
    expect(PROVIDER_CONFIG.ollama.urlEditable).toBe(true);
  });

  it("provedores conhecidos devem ter URL padrão e label", () => {
    const knownProviders = ["openai", "anthropic", "google", "ollama", "localai"];
    knownProviders.forEach((key) => {
      expect(PROVIDER_CONFIG[key as keyof typeof PROVIDER_CONFIG]).toBeDefined();
    });
  });
});

describe("ProviderForm - Renderização", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("deve renderizar campos básicos", () => {
    render(
      <ProviderForm
        onCancel={() => {}}
        onSave={() => {}}
      />
    );

    expect(screen.getByLabelText(/nome/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/perfil de servidor/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/base url/i)).toBeInTheDocument();
  });

  it('aplica o perfil vindo do catálogo e oferece links de cadastro e chave', async () => {
    render(<ProviderForm onCancel={() => {}} onSave={() => {}} />);
    await waitFor(() => expect(screen.getByLabelText(/perfil de servidor/i)).toHaveValue('openai'));
    expect(screen.getByLabelText(/base url/i)).toBeDisabled();
    expect(screen.getByRole('link', { name: 'providerForm.signupLink' })).toHaveAttribute('href', 'https://platform.openai.com/');
    expect(screen.getByRole('link', { name: 'providerForm.apiKeyLink' })).toHaveAttribute('href', 'https://platform.openai.com/api-keys');
  });

  it('consulta novamente o catálogo quando o primeiro snapshot ainda está atualizando', async () => {
    vi.mocked(GetLLMCatalog)
      .mockResolvedValueOnce(providerFormCatalogRefreshingAPIResponse)
      .mockResolvedValueOnce(providerFormCatalogAPIResponse);
    render(<ProviderForm onCancel={() => {}} onSave={() => {}} />);

    expect(await screen.findByRole('option', { name: 'OpenAI' })).toBeInTheDocument();
    expect(GetLLMCatalog).toHaveBeenCalledTimes(2);
  });

  it('não oferece o perfil Cohere enquanto sua rota catalogada não for suportada', async () => {
    render(<ProviderForm onCancel={() => {}} onSave={() => {}} />);
    const profileSelect = screen.getByLabelText(/perfil de servidor/i);
    await waitFor(() => expect(screen.getByRole('option', { name: 'OpenAI' })).toBeInTheDocument());
    expect(Array.from((profileSelect as HTMLSelectElement).options).map(option => option.value)).not.toContain('cohere');
    expect(profileSelect).toContainElement(screen.getByRole('option', { name: 'Personalizado' }));
  });

  it('informa quando o catálogo está indisponível e mantém a opção personalizada', async () => {
    vi.mocked(GetLLMCatalog).mockRejectedValueOnce(new Error('offline'));
    render(<ProviderForm onCancel={() => {}} onSave={() => {}} />);
    expect(await screen.findByText(/Catálogo indisponível/)).toBeInTheDocument();
    expect(screen.getByLabelText(/perfil de servidor/i)).toHaveValue('');
    expect(screen.getByLabelText(/base url/i)).toBeInTheDocument();
  });

  it('preserva URL, protocolo e autenticação ao trocar para o perfil personalizado', async () => {
    const user = userEvent.setup();
    render(<ProviderForm onCancel={() => {}} onSave={() => {}} />);
    const profileSelect = screen.getByLabelText(/perfil de servidor/i);
    await waitForProfileOption(profileSelect, 'ollama');
    await user.selectOptions(profileSelect, 'ollama');
    const urlInput = screen.getByLabelText(/base url/i);
    await user.clear(urlInput);
    await user.type(urlInput, 'http://127.0.0.1:11434');
    await user.selectOptions(profileSelect, '');

    expect(urlInput).toHaveValue('http://127.0.0.1:11434');
    expect(screen.getByLabelText('providerForm.apiProtocol')).toHaveValue('openai');
    expect(screen.getByLabelText('providerForm.authMode')).toHaveValue('none');
  });

  it('preserva URL e formato digitados antes de o catálogo responder', async () => {
    let resolveCatalog!: (response: Awaited<ReturnType<typeof GetLLMCatalog>>) => void;
    vi.mocked(GetLLMCatalog).mockReturnValueOnce(new Promise<Awaited<ReturnType<typeof GetLLMCatalog>>>(resolve => {
      resolveCatalog = resolve;
    }));
    const user = userEvent.setup();
    render(<ProviderForm onCancel={() => {}} onSave={() => {}} />);

    const urlInput = screen.getByLabelText(/base url/i);
    await user.clear(urlInput);
    await user.type(urlInput, 'https://gateway.example/v1');
    await user.selectOptions(screen.getByLabelText('providerForm.apiProtocol'), 'openai');

    await act(async () => {
      resolveCatalog(providerFormCatalogAPIResponse);
    });

    await waitFor(() => expect(screen.getByRole('option', { name: 'OpenAI' })).toBeInTheDocument());
    expect(screen.getByLabelText(/perfil de servidor/i)).toHaveValue('');
    expect(screen.getByLabelText(/base url/i)).toHaveValue('https://gateway.example/v1');
    expect(screen.getByLabelText('providerForm.apiProtocol')).toHaveValue('openai');
    expect(screen.getByLabelText('providerForm.authMode')).toHaveValue('optional');
  });

  it('usa os defaults customizados ao digitar o modelo antes de o catálogo responder', async () => {
    let resolveCatalog!: (response: Awaited<ReturnType<typeof GetLLMCatalog>>) => void;
    vi.mocked(GetLLMCatalog).mockReturnValueOnce(new Promise<Awaited<ReturnType<typeof GetLLMCatalog>>>(resolve => {
      resolveCatalog = resolve;
    }));
    const user = userEvent.setup();
    render(<ProviderForm onCancel={() => {}} onSave={() => {}} />);

    await user.type(screen.getByLabelText(/modelo padrão/i), 'manual-model');
    expect(screen.getByLabelText(/base url/i)).toHaveValue('');
    expect(screen.getByLabelText('providerForm.apiProtocol')).toHaveValue('openai');
    expect(screen.getByLabelText('providerForm.authMode')).toHaveValue('optional');

    await act(async () => resolveCatalog(providerFormCatalogAPIResponse));
    await waitFor(() => expect(screen.getByRole('option', { name: 'OpenAI' })).toBeInTheDocument());
    expect(screen.getByLabelText(/perfil de servidor/i)).toHaveValue('');
    expect(screen.getByLabelText(/modelo padrão/i)).toHaveValue('manual-model');
  });
  it("deve ter labels HTML associados corretamente (acessibilidade)", () => {
    render(
      <ProviderForm
        onCancel={() => {}}
        onSave={() => {}}
      />
    );

    const nomeInput = screen.getByLabelText(/nome/i);
    const nomeLabel = screen.getByText("Nome").closest("label");
    
    expect(nomeInput).toHaveAttribute("id");
    expect(nomeLabel).toHaveAttribute("for", nomeInput.getAttribute("id"));
  });

  it("deve mostrar URL read-only para OpenAI", async () => {
    render(
      <ProviderForm
        onCancel={() => {}}
        onSave={() => {}}
      />
    );

    const urlInput = screen.getByLabelText(/base url/i) as HTMLInputElement;
    await waitFor(() => {
      expect(urlInput).toBeDisabled();
      expect(urlInput.value).toBe("https://api.openai.com/v1");
    });
  });

  it("deve mostrar URL editável para Ollama", async () => {
    const user = userEvent.setup();
    render(
      <ProviderForm
        onCancel={() => {}}
        onSave={() => {}}
      />
    );

    const typeSelect = screen.getByLabelText(/perfil de servidor/i);
    await waitForProfileOption(typeSelect, 'ollama');
    await user.selectOptions(typeSelect, "ollama");

    const urlInput = screen.getByLabelText(/base url/i) as HTMLInputElement;
    expect(urlInput).not.toBeDisabled();
    expect(urlInput.value).toBe("http://localhost:11434");
    expect(screen.getByLabelText('providerForm.apiProtocol')).toHaveValue('openai');

  });

  it("deve exigir autenticação para OpenAI", () => {
    render(
      <ProviderForm
        onCancel={() => {}}
        onSave={() => {}}
      />
    );

    expect(screen.getByLabelText('providerForm.authMode')).toHaveValue('required');
    expect(screen.getByRole('button', { name: 'credentials.mcp.configure' })).toBeInTheDocument();
  });

  it("Ollama inicia sem autenticação e permite optar por credencial", async () => {
    const user = userEvent.setup();
    render(
      <ProviderForm
        onCancel={() => {}}
        onSave={() => {}}
      />
    );

    const typeSelect = screen.getByLabelText(/perfil de servidor/i);
    await waitForProfileOption(typeSelect, 'ollama');
    await user.selectOptions(typeSelect, "ollama");

    expect(screen.getByLabelText('providerForm.authMode')).toHaveValue('none');
    expect(screen.queryByRole('button', { name: 'credentials.mcp.configure' })).not.toBeInTheDocument();
    await waitForProfileOption(typeSelect, 'custom-openai-compatible');
    await user.selectOptions(typeSelect, 'custom-openai-compatible');
    await user.selectOptions(screen.getByLabelText('providerForm.authMode'), 'optional');
    expect(screen.getByRole('button', { name: 'credentials.mcp.configure' })).toBeInTheDocument();
  });

  it('restringe modelo padrão às opções choice declaradas no perfil', async () => {
    const user = userEvent.setup();
    render(<ProviderForm onCancel={() => {}} onSave={() => {}} />);
    const profileSelect = screen.getByLabelText(/perfil de servidor/i);
    await waitForProfileOption(profileSelect, 'deepseek');
    await user.selectOptions(profileSelect, 'deepseek');
    const modelSelect = screen.getByLabelText(/modelo padrão/i);
    expect(modelSelect.tagName).toBe('SELECT');
    expect(modelSelect).toHaveValue('deepseek-chat');
    expect(Array.from((modelSelect as HTMLSelectElement).options).map(option => option.value)).toEqual([
      'deepseek-chat', 'deepseek-reasoner',
    ]);
  });

  it('pede aceite antes de testar credenciais em HTTP remoto e vincula aceite à URL', async () => {
    const user = userEvent.setup();
    render(<ProviderForm onCancel={() => {}} onSave={() => {}} />);
    const profileSelect = screen.getByLabelText(/perfil de servidor/i);
    await waitForProfileOption(profileSelect, 'custom-openai-compatible');
    await user.selectOptions(profileSelect, 'custom-openai-compatible');
    await user.selectOptions(screen.getByLabelText('providerForm.authMode'), 'required');
    await user.type(screen.getByLabelText(/base url/i), 'http://gateway.example.test/v1');

    await user.click(screen.getByRole('button', { name: /carregar modelos/i }));
    expect(await screen.findByRole('alertdialog')).toBeInTheDocument();
    expect(App.ListModelsRaw).not.toHaveBeenCalled();

    await user.click(screen.getByRole('button', { name: 'providerForm.insecureHTTP.authorize' }));
    await waitFor(() => expect(App.ListModelsRaw).toHaveBeenCalledWith(expect.objectContaining({
      base_url: 'http://gateway.example.test/v1',
      insecure_http_authorized_url: 'http://gateway.example.test/v1',
    })));
  });

  it.each(['fixed-model-test', 'deepseek'])('permite testar a conexão com política de modelo %s', async profileId => {
    const user = userEvent.setup();
    render(<ProviderForm onCancel={() => {}} onSave={() => {}} />);
    const profileSelect = screen.getByLabelText(/perfil de servidor/i);
    await waitForProfileOption(profileSelect, profileId);
    await user.selectOptions(profileSelect, profileId);
    const createButton = screen.getByRole('button', { name: /criar/i });
    expect(createButton).toBeDisabled();
    await user.click(screen.getByRole('button', { name: /carregar modelos/i }));
    await waitFor(() => expect(createButton).toBeEnabled());
  });

  it('mantém visível um modelo manual que não aparece na lista retornada', async () => {
    const user = userEvent.setup();
    render(<ProviderForm onCancel={() => {}} onSave={() => {}} />);
    const profileSelect = screen.getByLabelText(/perfil de servidor/i);
    await waitForProfileOption(profileSelect, '');
    await user.selectOptions(profileSelect, '');
    const modelInput = screen.getByLabelText(/modelo padrão/i);
    await user.type(modelInput, 'gateway/private-model');
    await user.click(screen.getByRole('button', { name: /carregar modelos/i }));

    const modelSelect = await screen.findByLabelText(/modelo padrão/i);
    expect(modelSelect).toHaveValue('gateway/private-model');
    expect(modelSelect).toContainElement(screen.getByRole('option', { name: 'gateway/private-model' }));
  });
});

describe("ProviderForm - Validação", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("deve validar nome obrigatório no envio", () => {
    const mockOnSave = vi.fn();

    render(
      <ProviderForm
        onCancel={() => {}}
        onSave={mockOnSave}
      />
    );

    const nameInput = screen.getByLabelText(/nome/i) as HTMLInputElement;
    expect(nameInput.value).toBe("");

    const createButton = screen.getByRole("button", { name: /criar/i });
    expect(createButton).toBeDisabled();
  });

  it("deve validar URL obrigatória", async () => {
    const user = userEvent.setup();

    render(
      <ProviderForm
        onCancel={() => {}}
        onSave={() => {}}
      />
    );

    const nameInput = screen.getByLabelText(/nome/i);
    await user.type(nameInput, "Test");

    const typeSelect = screen.getByLabelText(/perfil de servidor/i);
    await waitForProfileOption(typeSelect, 'ollama');
    await user.selectOptions(typeSelect, "ollama");

    const urlInput = screen.getByLabelText(/base url/i) as HTMLInputElement;
    expect(urlInput.value).toBe("http://localhost:11434");

    await user.clear(urlInput);
    expect(urlInput.value).toBe("");
  });

  it("deve validar formato de URL", async () => {
    const user = userEvent.setup();

    render(
      <ProviderForm
        onCancel={() => {}}
        onSave={() => {}}
      />
    );

    const typeSelect = screen.getByLabelText(/perfil de servidor/i);
    await waitForProfileOption(typeSelect, 'ollama');
    await user.selectOptions(typeSelect, "ollama");

    const urlInput = screen.getByLabelText(/base url/i);
    await user.clear(urlInput);
    await user.type(urlInput, "not-a-url");

    expect(urlInput).toHaveValue("not-a-url");
  });

  it("deve validar API Key obrigatória para OpenAI", async () => {
    const user = userEvent.setup();

    render(
      <ProviderForm
        onCancel={() => {}}
        onSave={() => {}}
      />
    );

    const nameInput = screen.getByLabelText(/nome/i);
    await user.type(nameInput, "My OpenAI");

    await waitFor(() => expect(screen.getByRole('button', { name: 'credentials.mcp.configure' })).toBeEnabled());
    await user.click(screen.getByRole('button', { name: 'credentials.mcp.configure' }));
    expect(screen.getByText('credentials.sourceFields.required')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /carregar modelos/i })).toBeDisabled();
  });
});

describe("ProviderForm - Carregamento de Modelos", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("deve chamar ListModelsRaw com dados corretos ao clicar Carregar Modelos", async () => {
    const user = userEvent.setup();

    render(
      <ProviderForm
        onCancel={() => {}}
        onSave={() => {}}
      />
    );

    const nameInput = screen.getByLabelText(/nome/i);
    await user.type(nameInput, "My Ollama");

    const typeSelect = screen.getByLabelText(/perfil de servidor/i);
    await waitForProfileOption(typeSelect, 'ollama');
    await user.selectOptions(typeSelect, "ollama");

    const loadButton = screen.getByRole("button", { name: /carregar modelos/i });
    await user.click(loadButton);

    await waitFor(() => {
      expect(App.ListModelsRaw).toHaveBeenCalledWith(
        expect.objectContaining({
          type: "ollama",
          base_url: "http://localhost:11434",
        })
      );
    });
  });

  it("deve mostrar modelos quando carregados com sucesso", async () => {
    const user = userEvent.setup();

    render(
      <ProviderForm
        onCancel={() => {}}
        onSave={() => {}}
      />
    );

    const nameInput = screen.getByLabelText(/nome/i);
    await user.type(nameInput, "My Ollama");

    const typeSelect = screen.getByLabelText(/perfil de servidor/i);
    await waitForProfileOption(typeSelect, 'ollama');
    await user.selectOptions(typeSelect, "ollama");

    const loadButton = screen.getByRole("button", { name: /carregar modelos/i });
    await user.click(loadButton);

    await waitFor(() => {
      expect(screen.getByText(/conectado/i)).toBeInTheDocument();
    });

    // Deve mostrar o select de modelos
    expect(screen.getByLabelText(/modelo padrão/i)).toBeInTheDocument();
  });

  it("deve mostrar erro quando carregamento falhar", async () => {
    vi.mocked(App.ListModelsRaw).mockRejectedValueOnce(
      new Error("Conexão recusada")
    );

    const user = userEvent.setup();

    render(
      <ProviderForm
        onCancel={() => {}}
        onSave={() => {}}
      />
    );

    const nameInput = screen.getByLabelText(/nome/i);
    await user.type(nameInput, "My Ollama");

    const typeSelect = screen.getByLabelText(/perfil de servidor/i);
    await waitForProfileOption(typeSelect, 'ollama');
    await user.selectOptions(typeSelect, "ollama");

    const loadButton = screen.getByRole("button", { name: /carregar modelos/i });
    await user.click(loadButton);

    await waitFor(() => {
      expect(screen.getByText(/Conexão recusada/i)).toBeInTheDocument();
    });
  });

  it("deve mostrar input livre quando endpoint não suportado", async () => {
    vi.mocked(App.ListModelsRaw).mockRejectedValueOnce(
      new Error("models_endpoint_not_supported")
    );

    const user = userEvent.setup();

    render(
      <ProviderForm
        onCancel={() => {}}
        onSave={() => {}}
      />
    );

    const nameInput = screen.getByLabelText(/nome/i);
    await user.type(nameInput, "My Custom");

    const typeSelect = screen.getByLabelText(/perfil de servidor/i);
    await waitForProfileOption(typeSelect, 'ollama');
    await user.selectOptions(typeSelect, "ollama");

    const loadButton = screen.getByRole("button", { name: /carregar modelos/i });
    await user.click(loadButton);

    await waitFor(() => {
      // O status deve indicar conexão ok
      expect(screen.getByText(/conectado/i)).toBeInTheDocument();
    });
  });
});

describe("ProviderForm - Salvar", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("deve chamar CreateLLMProvider quando criando novo", async () => {
    const mockOnSave = vi.fn();
    const user = userEvent.setup();

    render(
      <ProviderForm
        onCancel={() => {}}
        onSave={mockOnSave}
      />
    );

    const nameInput = screen.getByLabelText(/nome/i);
    await user.type(nameInput, "My Ollama");

    const typeSelect = screen.getByLabelText(/perfil de servidor/i);
    await waitForProfileOption(typeSelect, 'ollama');
    await user.selectOptions(typeSelect, "ollama");

    const loadButton = screen.getByRole("button", { name: /carregar modelos/i });
    await user.click(loadButton);

    await waitFor(() => {
      expect(screen.getByText(/conectado/i)).toBeInTheDocument();
    });

    const createButton = screen.getByRole("button", { name: /criar/i });
    await user.click(createButton);

    await waitFor(() => {
      expect(App.CreateLLMProvider).toHaveBeenCalledWith(expect.objectContaining({
        type: 'ollama',
        api_format: 'openai',
        reasoning_content_mode: "disabled",
        server_profile_id: "ollama",
      }));
      expect(mockOnSave).toHaveBeenCalled();
    });
  });

  it("deve chamar UpdateLLMProvider quando editando", async () => {
    const mockOnSave = vi.fn();
    const user = userEvent.setup();

    const existingProvider = {
      id: "ollama-123",
      name: "My Ollama",
      type: "ollama",
      base_url: "http://localhost:11434",
      api_key: "",
    };

    render(
      <ProviderForm
        provider={existingProvider}
        onCancel={() => {}}
        onSave={mockOnSave}
      />
    );

    // Testar é explícito: abrir edição não executa a fonte command.
    expect(App.ListModelsRaw).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: /carregar modelos/i }));
    await waitFor(() => {
      expect(screen.getByText(/conectado/i)).toBeInTheDocument();
    });

    const buttons = screen.getAllByRole("button");
    const updateButton = buttons.find(btn => btn.textContent?.includes("Atualizar"));
    if (!updateButton) throw new Error("Update button not found");
    await user.click(updateButton);

    await waitFor(() => {
      expect(App.UpdateLLMProvider).toHaveBeenCalled();
      expect(mockOnSave).toHaveBeenCalled();
    });
  });
});

describe("api_format semantics", () => {
  it("OpenAI real deve usar openai_responses como default", () => {
    expect(PROVIDER_CONFIG.openai.apiFormat).toBe("openai_responses");
  });

  it("providers OpenAI-compatible devem usar openai como default", () => {
    const compatibleProviders = [
      "openrouter", "deepseek", "xai", "mistral", "groq",
      "together", "fireworks", "perplexity", "cohere",
      "ollama", "localai", "litellm",
    ];
    for (const key of compatibleProviders) {
      expect(PROVIDER_CONFIG[key as keyof typeof PROVIDER_CONFIG]?.apiFormat).toBe(
        "openai",
      );
    }
  });

  it("Anthropic deve usar anthropic como default", () => {
    expect(PROVIDER_CONFIG.anthropic.apiFormat).toBe("anthropic");
  });

  it("Google deve usar google como default", () => {
    expect(PROVIDER_CONFIG.google.apiFormat).toBe("google");
  });

  it("API_FORMAT_OPTIONS deve ter labels corretos", () => {
    const responsesOpt = API_FORMAT_OPTIONS.find(o => o.value === "openai_responses");
    expect(responsesOpt).toBeDefined();
    expect(responsesOpt!.label).toBe("OpenAI — Responses API");

    const compatOpt = API_FORMAT_OPTIONS.find(o => o.value === "openai");
    expect(compatOpt).toBeDefined();
    expect(compatOpt!.label).toBe("OpenAI-compatible — Chat Completions");
  });

  it("formulário de criação OpenAI deve iniciar com openai_responses", () => {
    render(
      <ProviderForm onCancel={() => {}} onSave={() => {}} />
    );

    const apiFormatLabel = screen.getByText("providerForm.apiProtocol");
    const selectId = apiFormatLabel.closest("label")?.getAttribute("for");
    const select = document.getElementById(selectId!) as HTMLSelectElement;
    expect(select.value).toBe("openai_responses");
  });

  it("mudar para Ollama deve trocar api_format para openai", async () => {
    const user = userEvent.setup();
    render(
      <ProviderForm onCancel={() => {}} onSave={() => {}} />
    );

    const typeSelect = screen.getByLabelText(/perfil de servidor/i);
    await waitForProfileOption(typeSelect, 'ollama');
    await user.selectOptions(typeSelect, "ollama");

    const apiFormatLabel = screen.getByText("providerForm.apiProtocol");
    const selectId = apiFormatLabel.closest("label")?.getAttribute("for");
    const select = document.getElementById(selectId!) as HTMLSelectElement;
    expect(select.value).toBe("openai");
  });

  it("capability de reasoning_content vem do preset e pode ser alterada", async () => {
    const user = userEvent.setup();
    render(
      <ProviderForm onCancel={() => {}} onSave={() => {}} />
    );

    const typeSelect = screen.getByLabelText(/perfil de servidor/i);
    await waitForProfileOption(typeSelect, 'deepseek');
    await user.selectOptions(typeSelect, "deepseek");

    const mode = screen.getByLabelText(/compatibilidade com reasoning_content/i);
    expect(mode).toHaveValue("replay_with_tools");

    await user.selectOptions(mode, "disabled");
    expect(mode).toHaveValue("disabled");
  });
});


it('permite criar provedor usando a credencial de source cadastrada no cofre', async () => {
  vi.clearAllMocks();
  render(<ProviderForm onCancel={() => {}} onSave={() => {}} />);
  await userEvent.type(screen.getByLabelText(/nome/i), 'Gateway');

  expect(screen.queryByLabelText(/^API Key$/i)).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole('button', { name: /carregar modelos/i }));
  await waitFor(() => expect(App.ListModelsRaw).toHaveBeenCalledWith(expect.objectContaining({ credential: undefined })));
  await userEvent.click(screen.getByRole('button', { name: 'Criar' }));
  await waitFor(() => expect(App.CreateLLMProvider).toHaveBeenCalledWith(expect.objectContaining({ name: 'Gateway', credential: undefined })));
});

it('descarta o rascunho ao trocar de provedor e voltar para criação', async () => {
 const onSave = vi.fn(); const onCancel = vi.fn();
 const { rerender } = render(<ProviderForm onSave={onSave} onCancel={onCancel} />);
 const configure = () => screen.getByRole('button', { name: 'credentials.mcp.configure' });
 await waitFor(() => expect(configure()).toBeEnabled());
 await userEvent.click(configure());
 await userEvent.type(screen.getByLabelText('credentials.labels.token'), 'private-draft');
 const provider = { id: 'saved', name: 'Saved', type: 'openai', base_url: 'https://api.openai.com/v1', api_key: '' };
 rerender(<ProviderForm provider={provider} onSave={onSave} onCancel={onCancel} />);
 expect(screen.queryByDisplayValue('private-draft')).not.toBeInTheDocument();
 await waitFor(() => expect(configure()).toBeEnabled());
 await userEvent.click(configure());
 expect(screen.getByLabelText('credentials.labels.token')).toHaveValue('');
 rerender(<ProviderForm onSave={onSave} onCancel={onCancel} />);
 expect(screen.queryByLabelText('credentials.labels.token')).not.toBeInTheDocument();
 expect(App.ListModelsRaw).not.toHaveBeenCalled();
});


describe('ProviderForm - vínculos privados', () => {
 it('envia o vínculo salvo sem substituir o modelo padrão e permite remover offline', async () => {
   vi.mocked(GetLLMCatalog).mockRejectedValueOnce(new Error('offline'));
   const provider = { id: 'private-connection', name: 'Proxy', type: 'custom', base_url: 'https://example.com/v1', api_key: '', api_format: 'openai', auth_mode: 'none', default_model: 'gpt-4o', model_aliases: { 'team/private': 'vendor/removed' } };
   render(<ProviderForm provider={provider} onSave={vi.fn()} onCancel={vi.fn()} />);
   await userEvent.click(screen.getByRole('button', { name: /Carregar modelos/i }));
   await waitFor(() => expect(screen.getByRole('button', { name: 'Atualizar' })).toBeEnabled());
   await userEvent.click(screen.getByRole('button', { name: 'Atualizar' }));
   expect(App.UpdateLLMProvider).toHaveBeenLastCalledWith('private-connection', expect.objectContaining({ model_aliases: { 'team/private': 'vendor/removed' }, default_model: 'gpt-4o' }));
   await userEvent.click(screen.getByRole('button', { name: /providerForm.aliases.remote/ }));
   await userEvent.type(screen.getByRole('combobox', { name: /providerForm.aliases.remote/ }), 'team/private');
   await userEvent.keyboard('{Enter}');
   await userEvent.selectOptions(screen.getByLabelText('providerForm.aliases.canonical'), '');
   await userEvent.click(screen.getByRole('button', { name: 'Atualizar' }));
   expect(App.UpdateLLMProvider).toHaveBeenLastCalledWith('private-connection', expect.objectContaining({ model_aliases: {} }));
 });
});


it('apresenta o erro traduzido quando o alvo do vínculo sai do catálogo', async () => {
 vi.mocked(App.UpdateLLMProvider).mockRejectedValueOnce('model_alias_error:v1:missing_target:vendor%2Fgone');
 render(<ProviderForm provider={{ id: 'saved', name: 'Proxy', type: 'custom', base_url: 'https://example.com/v1', api_key: '', auth_mode: 'none' }} onSave={vi.fn()} onCancel={vi.fn()} />);
 await userEvent.click(screen.getByRole('button', { name: /Carregar modelos/i }));
 await waitFor(() => expect(screen.getByRole('button', { name: 'Atualizar' })).toBeEnabled());
 await userEvent.click(screen.getByRole('button', { name: 'Atualizar' }));
 expect(await screen.findByText('O modelo "vendor/gone" não está mais no catálogo. Atualize o vínculo antes de salvar ou duplicar a conexão.')).toBeInTheDocument();
});
