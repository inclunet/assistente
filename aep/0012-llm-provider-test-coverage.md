# Test Coverage - Provider System

## Status Geral

O sistema de provedores LLM foi implementado com cobertura de testes **parcial**. Enquanto operações CRUD (Create, Read, Update, Delete) têm testes completos, a validação de conexão e a interface de usuário ainda carecem de testes unitários.

## ✅ Testes Existentes

### Backend (Go) - `app_provider_crud_test.go`

Arquivo: [app_provider_crud_test.go](../app_provider_crud_test.go)

**Testes Implementados:**

1. **TestCreateProviderWithAPIKey** ✅
   - Valida criação de provedor com API Key
   - Verifica armazenamento de credenciais cifradas
   - Confirma padrão de URL do provedor
   - Status: PASSANDO

2. **TestUpdateProvider** ✅
   - Testa atualização de detalhes do provedor
   - Valida mudança de API Key
   - Status: PASSANDO

3. **TestDeleteProvider** ✅
   - Verifica remoção de provedor do registro
   - Confirma deleção de credenciais associadas
   - Status: PASSANDO

4. **TestListProvidersWithStatus** ✅
   - Lista provedores com indicador de status
   - Mostra se credencial está configurada
   - Status: PASSANDO

**Cobertura CRUD:** 100% ✅

### Frontend (React/TypeScript) - Vitest

**Status:** Sem testes específicos para ProviderForm.tsx

## ❌ Testes Faltando

### 1. Backend - TestLLMProvider (HTTP Validation)

**Função:** [app.go](../app.go) linhas ~2849-2900

**O que testa:** Validação de conexão com provedor LLM via HTTP GET

**Testes que deveriam existir:**
- ✗ Validação de URL obrigatória
- ✗ Validação de formato de URL
- ✗ Conexão bem-sucedida (HTTP 200)
- ✗ Erro de autenticação (HTTP 401)
- ✗ Erro do servidor (HTTP 500)
- ✗ Tratamento de timeout
- ✗ Suporte a Auth Bearer Token
- ✗ Funcionamento sem API Key (provedores locais)

**Complexidade:** ⭐⭐⭐ Média (requer mock HTTP server)

### 2. Frontend - ProviderForm Component

**Arquivo:** [ProviderForm.tsx](../frontend/src/components/settings/ProviderForm.tsx)

**O que testa:** Lógica completa da interface de formulário

**Testes que deveriam existir:**

**Configuração (PROVIDER_CONFIG):**
- ✗ Estrutura válida para cada tipo de provedor
- ✗ URLs corretas (comerciais fixas, locais editáveis)
- ✗ Requisitos de API Key (obrigatório vs opcional)

**Renderização:**
- ✗ Campos aparecem/desaparecem conforme tipo
- ✗ API Key é obrigatória para OpenAI
- ✗ API Key é opcional para Ollama
- ✗ URL é read-only para provedores comerciais
- ✗ URL é editável para provedores locais

**Validação:**
- ✗ Nome obrigatório
- ✗ URL obrigatória e com formato válido
- ✗ API Key obrigatória (quando configurado)
- ✗ Mensagens de erro apropriadas

**Testador de Conexão:**
- ✗ Chamada correta à função TestLLMProvider
- ✗ Estados de sucesso/erro
- ✗ Auto-teste ao sair do campo API Key
- ✗ Desabilitação do botão Salvar até testar

**Salvar Provedor:**
- ✗ Criação de novo provedor
- ✗ Atualização de provedor existente
- ✗ Tratamento de erros

**Acessibilidade:**
- ✗ Labels associadas aos inputs
- ✗ Atributos aria-required
- ✗ Atributos aria-invalid

**Complexidade:** ⭐⭐⭐⭐ Alta (requer complex mocking de Wails + Vitest)

## 📊 Cobertura Atual

```
Backend CRUD:           ✅ 100% (4 testes)
Backend Validação HTTP: ❌ 0% (0 testes)
Frontend Renderização:  ❌ 0% (0 testes)
Frontend Validação:     ❌ 0% (0 testes)
Frontend Integração:    ❌ 0% (0 testes)

Total Cobertura:        ~20% (4 de ~20 testes desejados)
```

## 🚀 Próximos Passos Recomendados

### Prioridade Alta (P0):
1. Adicionar testes para `TestLLMProvider()`
   - HTTP status codes
   - URL validation
   - Bearer token handling
   - Timeout scenarios

### Prioridade Média (P1):
2. Adicionar testes para ProviderForm component
   - Renderização condicional
   - Validação dinâmica por tipo
   - Estados do botão de teste/salvar
   - Mensagens de erro

### Prioridade Baixa (P2):
3. Integração E2E
   - Fluxo completo: criar provedor → testar → salvar
   - Verificação no banco de dados
   - Validação de UI updates

## 📝 Instruções para Adicionar Testes

### Backend (Go)

Criar novo arquivo: `app_test_llm_provider_validation_test.go`

```go
package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	
	"assistente/internal/credentials"
	"assistente/internal/llm"
)

func TestTestLLMProviderValidatesUrl(t *testing.T) {
	// Setup
	credMgr := credentials.NewManager([]byte("test-key-exactly-32-bytes-long!!"))
	llmRegistry := llm.NewProviderRegistry()
	app := &App{
		credMgr:     credMgr,
		llmRegistry: llmRegistry,
	}

	// Test com URL vazia
	req := TestLLMProviderRequest{
		Type:    "openai",
		BaseURL: "",
		APIKey:  "sk-test",
	}
	
	_, err := app.TestLLMProvider(context.Background(), req)
	if err == nil {
		t.Error("Expected error for empty URL")
	}
}
```

### Frontend (React/TypeScript)

Criar novo arquivo: `ProviderForm.test.tsx`

```tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import ProviderForm from "./ProviderForm";

// Mock Wails
vi.mock("@wailsjs/go/app/App", () => ({
  TestLLMProvider: vi.fn(),
  CreateLLMProvider: vi.fn(),
  UpdateLLMProvider: vi.fn(),
}));

describe("ProviderForm", () => {
  it("deve renderizar formulário com campos básicos", () => {
    render(
      <ProviderForm
        isOpen={true}
        onClose={() => {}}
        onSave={() => {}}
      />
    );

    expect(screen.getByLabelText("Nome do Provedor")).toBeInTheDocument();
  });
});
```

## 🔍 Conclusão

O sistema de provedores LLM está **funcional e testado para CRUD**, mas requer:
- Testes de validação HTTP (backend)
- Testes de interface (frontend)
- Testes de integração E2E

Recomenda-se adicionar testes conforme prioridade acima, começando com P0 (validação HTTP).

---

**Última Atualização:** 2026-03-07
**Responsável:** GitHub Copilot (assistente)
