import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { ProviderForm } from "./ProviderForm";

// Mock Wails API
vi.mock("@wailsjs/go/main/App", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@wailsjs/go/main/App")>();
  return {
    ...actual,
    TestLLMProvider: vi.fn(() => Promise.resolve(true)),
    CreateLLMProvider: vi.fn(() => Promise.resolve({ id: "123" })),
    UpdateLLMProvider: vi.fn(() => Promise.resolve({})),
  };
});

import * as App from "@wailsjs/go/main/App";

describe("ProviderForm - Edição de API Key", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(App.TestLLMProvider).mockResolvedValue(true);
  });

  it("deve mostrar botão 'Alterar Chave' ao editar provedor com key obrigatória", () => {
    render(
      <ProviderForm
        provider={{
          id: "openai-123",
          name: "OpenAI Prod",
          type: "openai",
          base_url: "https://api.openai.com/v1",
          api_key: "",
        }}
        onCancel={() => {}}
        onSave={() => {}}
      />
    );

    // Deve mostrar botão de alterar, sem mostrar campo de input
    expect(screen.getByText(/🔓 Alterar Chave/i)).toBeInTheDocument();
    // O label "API Key" ainda existe, mas o input não está visível
    const apiKeyButtons = screen.getAllByRole("button").filter(btn => btn.textContent?.includes("Alterar"));
    expect(apiKeyButtons.length).toBeGreaterThan(0);
  });

  it("deve mostrar descrição de key configurada quando em modo edição", () => {
    render(
      <ProviderForm
        provider={{
          id: "openai-123",
          name: "OpenAI Prod",
          type: "openai",
          base_url: "https://api.openai.com/v1",
          api_key: "",
        }}
        onCancel={() => {}}
        onSave={() => {}}
      />
    );

    expect(screen.getByText(/🔑 Chave configurada no gerenciador de credenciais/i)).toBeInTheDocument();
  });

  it("deve exibir campo de API Key ao clicar em 'Alterar Chave'", async () => {
    const user = userEvent.setup();
    render(
      <ProviderForm
        provider={{
          id: "openai-123",
          name: "OpenAI Prod",
          type: "openai",
          base_url: "https://api.openai.com/v1",
          api_key: "",
        }}
        onCancel={() => {}}
        onSave={() => {}}
      />
    );

    const changeButton = screen.getByText(/🔓 Alterar Chave/i);
    await user.click(changeButton);

    expect(screen.getByLabelText(/api key/i)).toBeInTheDocument();
    expect(screen.queryByText(/🔓 Alterar Chave/i)).not.toBeInTheDocument();
  });

  it("deve exibir campo de API Key imediatamente ao criar novo provedor", () => {
    render(
      <ProviderForm
        onCancel={() => {}}
        onSave={() => {}}
      />
    );

    expect(screen.getByLabelText(/api key/i)).toBeInTheDocument();
    expect(screen.queryByText(/🔓 Alterar Chave/i)).not.toBeInTheDocument();
  });

  it("deve mostrar botão 'Alterar Chave' para provedores com key opcional", () => {
    render(
      <ProviderForm
        provider={{
          id: "ollama-123",
          name: "Ollama Local",
          type: "ollama",
          base_url: "http://localhost:11434",
          api_key: "",
        }}
        onCancel={() => {}}
        onSave={() => {}}
      />
    );

    expect(screen.getByText(/🔓 Alterar Chave/i)).toBeInTheDocument();
  });

  it("deve manter key oculta após alterar tipo de provedor se já estava em modo edição", async () => {
    const user = userEvent.setup();
    render(
      <ProviderForm
        provider={{
          id: "openai-123",
          name: "OpenAI Prod",
          type: "openai",
          base_url: "https://api.openai.com/v1",
          api_key: "",
        }}
        onCancel={() => {}}
        onSave={() => {}}
      />
    );

    // Inicialmente mostra botão
    expect(screen.getByText(/🔓 Alterar Chave/i)).toBeInTheDocument();

    // Alterar tipo (não deve mostrar campo automaticamente)
    const typeSelect = screen.getByLabelText(/tipo/i);
    await user.selectOptions(typeSelect, "anthropic");

    // Ainda deve mostrar botão (não expôr campo automaticamente)
    expect(screen.getByText(/🔓 Alterar Chave/i)).toBeInTheDocument();
    // O input do tipo password não deve estar visível
    const passwordInputs = screen.queryAllByDisplayValue("");
    const apiKeyInput = passwordInputs.find(input => (input as HTMLInputElement).type === "password");
    expect(apiKeyInput).toBeUndefined();
  });
});

describe("ProviderForm - Restrições de URL em Edição", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(App.TestLLMProvider).mockResolvedValue(true);
  });

  it("não deve permitir editar URL de OpenAI mesmo em modo edição", () => {
    render(
      <ProviderForm
        provider={{
          id: "openai-123",
          name: "OpenAI Prod",
          type: "openai",
          base_url: "https://api.openai.com/v1",
          api_key: "",
        }}
        onCancel={() => {}}
        onSave={() => {}}
      />
    );

    const urlInput = screen.getByLabelText(/base url/i) as HTMLInputElement;
    expect(urlInput).toBeDisabled();
    expect(urlInput).toHaveAttribute("readonly");
    expect(urlInput.value).toBe("https://api.openai.com/v1");
  });

  it("não deve permitir editar URL de Anthropic mesmo em modo edição", () => {
    render(
      <ProviderForm
        provider={{
          id: "anthropic-123",
          name: "Anthropic Prod",
          type: "anthropic",
          base_url: "https://api.anthropic.com",
          api_key: "",
        }}
        onCancel={() => {}}
        onSave={() => {}}
      />
    );

    const urlInput = screen.getByLabelText(/base url/i) as HTMLInputElement;
    expect(urlInput).toBeDisabled();
    expect(urlInput).toHaveAttribute("readonly");
  });

  it("não deve permitir editar URL de Google mesmo em modo edição", () => {
    render(
      <ProviderForm
        provider={{
          id: "google-123",
          name: "Google Gemini",
          type: "google",
          base_url: "https://generativelanguage.googleapis.com",
          api_key: "",
        }}
        onCancel={() => {}}
        onSave={() => {}}
      />
    );

    const urlInput = screen.getByLabelText(/base url/i) as HTMLInputElement;
    expect(urlInput).toBeDisabled();
  });

  it("deve permitir editar URL de Ollama em modo edição", () => {
    render(
      <ProviderForm
        provider={{
          id: "ollama-123",
          name: "Ollama Remoto",
          type: "ollama",
          base_url: "http://192.168.1.100:11434",
          api_key: "",
        }}
        onCancel={() => {}}
        onSave={() => {}}
      />
    );

    const urlInput = screen.getByLabelText(/base url/i) as HTMLInputElement;
    expect(urlInput).not.toBeDisabled();
    expect(urlInput).not.toHaveAttribute("readonly");
    expect(urlInput.value).toBe("http://192.168.1.100:11434");
  });

  it("deve permitir editar URL de LiteLLM em modo edição", () => {
    render(
      <ProviderForm
        provider={{
          id: "litellm-123",
          name: "LiteLLM Proxy",
          type: "litellm",
          base_url: "http://localhost:4000",
          api_key: "",
        }}
        onCancel={() => {}}
        onSave={() => {}}
      />
    );

    const urlInput = screen.getByLabelText(/base url/i) as HTMLInputElement;
    expect(urlInput).not.toBeDisabled();
    expect(urlInput).not.toHaveAttribute("readonly");
  });

  it("deve permitir editar URL de LocalAI em modo edição", () => {
    render(
      <ProviderForm
        provider={{
          id: "localai-123",
          name: "LocalAI",
          type: "localai",
          base_url: "http://localhost:8080",
          api_key: "",
        }}
        onCancel={() => {}}
        onSave={() => {}}
      />
    );

    const urlInput = screen.getByLabelText(/base url/i) as HTMLInputElement;
    expect(urlInput).not.toBeDisabled();
  });

  it("deve bloquear URL ao mudar para provedor não editável em modo edição", async () => {
    const user = userEvent.setup();
    render(
      <ProviderForm
        provider={{
          id: "ollama-123",
          name: "Meu Provedor",
          type: "ollama",
          base_url: "http://localhost:11434",
          api_key: "",
        }}
        onCancel={() => {}}
        onSave={() => {}}
      />
    );

    // URL inicialmente editável
    let urlInput = screen.getByLabelText(/base url/i) as HTMLInputElement;
    expect(urlInput).not.toBeDisabled();

    // Mudar para OpenAI
    const typeSelect = screen.getByLabelText(/tipo/i);
    await user.selectOptions(typeSelect, "openai");

    // URL agora deve estar bloqueada, mas valor ainda é o antigo (não muda automaticamente em edição)
    urlInput = screen.getByLabelText(/base url/i) as HTMLInputElement;
    expect(urlInput).toBeDisabled();
    // Em modo edição, URL não muda automaticamente quando tipo muda
    // (para evitar perda acidental de dados)
  });
});
