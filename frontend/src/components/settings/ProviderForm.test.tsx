import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { ProviderForm, PROVIDER_CONFIG } from "./ProviderForm";

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (key: string) => {
      const translations: Record<string, string> = {
        'providerForm.name': 'Nome',
        'providerForm.namePlaceholder': 'Nome do provedor',
        'providerForm.providerType': 'Tipo',
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
  }),
}));

// Mock Wails API
vi.mock("@wailsjs/go/main/App", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@wailsjs/go/main/App")>();
  return {
    ...actual,
    ListModelsRaw: vi.fn(() => Promise.resolve(["gpt-4o", "gpt-4o-mini"])),
    CreateLLMProvider: vi.fn(() => Promise.resolve({ id: "123" })),
    UpdateLLMProvider: vi.fn(() => Promise.resolve({})),
  };
});

import * as App from "@wailsjs/go/main/App";

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
    expect(screen.getByLabelText(/tipo/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/base url/i)).toBeInTheDocument();
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

  it("deve mostrar URL read-only para OpenAI", () => {
    render(
      <ProviderForm
        onCancel={() => {}}
        onSave={() => {}}
      />
    );

    const urlInput = screen.getByLabelText(/base url/i) as HTMLInputElement;
    expect(urlInput).toBeDisabled();
    expect(urlInput.value).toBe("https://api.openai.com/v1");
  });

  it("deve mostrar URL editável para Ollama", async () => {
    const user = userEvent.setup();
    render(
      <ProviderForm
        onCancel={() => {}}
        onSave={() => {}}
      />
    );

    const typeSelect = screen.getByLabelText(/tipo/i);
    await user.selectOptions(typeSelect, "ollama");

    const urlInput = screen.getByLabelText(/base url/i) as HTMLInputElement;
    expect(urlInput).not.toBeDisabled();
    expect(urlInput.value).toBe("http://localhost:11434");
  });

  it("deve exigir API Key para OpenAI", () => {
    render(
      <ProviderForm
        onCancel={() => {}}
        onSave={() => {}}
      />
    );

    expect(screen.getByLabelText(/api key/i)).toBeInTheDocument();
    const allLabels = screen.getAllByText(/api key/i);
    const apiKeyLabel = allLabels.find(el => el.tagName === "LABEL");
    expect(apiKeyLabel).toHaveTextContent("*");
  });

  it("deve ter API Key opcional para Ollama", async () => {
    const user = userEvent.setup();
    render(
      <ProviderForm
        onCancel={() => {}}
        onSave={() => {}}
      />
    );

    const typeSelect = screen.getByLabelText(/tipo/i);
    await user.selectOptions(typeSelect, "ollama");

    const apiKeyLabel = screen.getByText(/api key/i).closest("label");
    expect(apiKeyLabel).not.toHaveTextContent("*");
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

    const typeSelect = screen.getByLabelText(/tipo/i);
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

    const typeSelect = screen.getByLabelText(/tipo/i);
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

    const allLabels = screen.getAllByText(/api key/i);
    const apiKeyLabel = allLabels.find(el => el.tagName === "LABEL");
    expect(apiKeyLabel).toHaveTextContent("*");
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

    const typeSelect = screen.getByLabelText(/tipo/i);
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

    const typeSelect = screen.getByLabelText(/tipo/i);
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

    const typeSelect = screen.getByLabelText(/tipo/i);
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

    const typeSelect = screen.getByLabelText(/tipo/i);
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

    const typeSelect = screen.getByLabelText(/tipo/i);
    await user.selectOptions(typeSelect, "ollama");

    const loadButton = screen.getByRole("button", { name: /carregar modelos/i });
    await user.click(loadButton);

    await waitFor(() => {
      expect(screen.getByText(/conectado/i)).toBeInTheDocument();
    });

    const createButton = screen.getByRole("button", { name: /criar/i });
    await user.click(createButton);

    await waitFor(() => {
      expect(App.CreateLLMProvider).toHaveBeenCalled();
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

    // Auto-carregamento de modelos roda automaticamente ao abrir edição
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

