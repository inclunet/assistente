# Plano de Implementação: Sistema de Email no Assistente

## 1. Visão Geral

Implementar suporte integrado para email no Assistente com:
- **Múltiplos provedores**: Gmail, Outlook, IMAP/SMTP genérico
- **Interface acessível**: Visualização, gerenciamento e resposta de emails
- **Integração com canais**: Email como um canal de comunicação
- **Gestão unificada**: Caixas de entrada, pastas, buscas e filtros em um único lugar

### Objetivos

1. ✅ Receber e sincronizar emails (IMAP)
2. ✅ Enviar emails (SMTP)
3. ✅ Autenticação segura (OAuth para Gmail/Outlook, credenciais para IMAP)
4. ✅ Interface accessível para leitura/resposta
5. ✅ Gerenciamento de múltiplas contas de email
6. ✅ Notificações de novos emails
7. ✅ Integração com contatos do Assistente

---

## 2. Arquitetura Geral

### 2.1 Fluxo de Dados

```
┌─────────────────────────────────────────────────────────────┐
│                    Frontend UI (Email Tab)                   │
│  - EmailListPage (caixa de entrada)                          │
│  - EmailDetailPage (visualizar email)                        │
│  - EmailComposePage (enviar/responder)                       │
│  - EmailSettingsPage (contas, pastas, filtros)              │
└────────────────────────┬────────────────────────────────────┘
                         │
                    Wails IPC
                         │
┌────────────────────────▼────────────────────────────────────┐
│            Backend Email Manager (Go)                        │
│  - AccountManager: gerencia contas de email                 │
│  - SyncEngine: sincroniza IMAP periodicamente               │
│  - MessageHandler: converte emails ↔ IncomingMessage        │
└────────────────────────┬────────────────────────────────────┘
                         │
        ┌────────────────┼────────────────┐
        │                │                │
    ┌───▼────┐      ┌───▼────┐      ┌───▼────┐
    │  IMAP  │      │  SMTP  │      │ Database│
    │ Gmail  │      │ Gmail  │      │ Emails  │
    │Outlook │      │Outlook │      │ Drafts  │
    │Generic │      │Generic │      │Settings │
    └────────┘      └────────┘      └────────┘
```

### 2.2 Componentes Principais

#### Backend (Go)

| Componente | Responsabilidade | Arquivos |
|-----------|------------------|----------|
| **EmailAccount** | Gerencia credenciais/tokens de uma conta | `internal/email/account.go` |
| **IMAPClient** | Conecta ao servidor IMAP, sincroniza caixa | `internal/email/imap_client.go` |
| **SMTPClient** | Conecta ao servidor SMTP, envia emails | `internal/email/smtp_client.go` |
| **OAuthManager** | Gerencia OAuth para Gmail/Outlook | `internal/email/oauth_manager.go` |
| **SyncEngine** | Sincronização periódica em background | `internal/email/sync_engine.go` |
| **MessageConverter** | Converte Email ↔ IncomingMessage | `internal/email/converter.go` |
| **EmailGateway** | Integração com messaging.Gateway (como Slack) | `internal/email/gateway.go` |
| **EmailStore** | Persistência de emails em SQLite | `internal/email/store.go` |

#### Frontend (React/TypeScript)

| Componente | Responsabilidade | Arquivo |
|-----------|------------------|---------|
| **EmailListPage** | Lista de emails (caixa entrada, pastas) | `frontend/src/pages/EmailListPage.tsx` |
| **EmailDetailPage** | Visualização completa de um email | `frontend/src/pages/EmailDetailPage.tsx` |
| **EmailComposePage** | Composer para novo email ou resposta | `frontend/src/pages/EmailComposePage.tsx` |
| **EmailSettingsPage** | Gerenciamento de contas e configurações | `frontend/src/pages/EmailSettingsPage.tsx` |
| **EmailThread** | Exibição de thread (email + respostas) | `frontend/src/components/EmailThread.tsx` |
| **EmailSearchBar** | Busca com filtros | `frontend/src/components/EmailSearchBar.tsx` |
| **OAuthCallbackHandler** | Recebe callback de Gmail/Outlook OAuth | `frontend/src/pages/OAuthCallback.tsx` |

---

## 3. Suporte a Provedores

### 3.1 Gmail (OAuth 2.0 + IMAP/SMTP)

**Fluxo de Autenticação:**
```
1. Usuário clica "Conectar Gmail" na UI
2. Frontend abre OAuth popup para accounts.google.com
3. Usuário autoriza (scopes: mail.google.com)
4. Backend recebe authorization_code
5. Backend troca por access_token + refresh_token
6. Armazena tokens de forma criptografada no DB
7. Usa IMAP/SMTP com access_token para enviar/receber
```

**Scopes necessários:**
- `https://www.googleapis.com/auth/gmail.readonly` - ler emails
- `https://www.googleapis.com/auth/gmail.send` - enviar emails
- `https://www.googleapis.com/auth/gmail.modify` - mover/arquivar

**Credenciais OAuth (console.cloud.google.com):**
- Client ID: xxxx.apps.googleusercontent.com
- Client Secret: guardado no backend
- Redirect URI: `http://localhost:6805/oauth/google/callback` (durante dev)

**Endpoints IMAP/SMTP:**
- IMAP: `imap.gmail.com:993` (SSL)
- SMTP: `smtp.gmail.com:587` (TLS)

### 3.2 Outlook/Microsoft 365 (OAuth 2.0 + IMAP/SMTP)

**Fluxo de Autenticação:**
```
1. Usuário clica "Conectar Outlook" na UI
2. Frontend abre OAuth popup para login.microsoftonline.com
3. Usuário autoriza (scopes: Mail.Read, Mail.Send)
4. Backend recebe authorization_code
5. Backend troca por access_token + refresh_token
6. Armazena tokens criptografados
7. Usa IMAP/SMTP com access_token
```

**Scopes necessários:**
- `Mail.Read` - ler emails
- `Mail.Send` - enviar emails
- `MailboxSettings.Read` - ler configurações

**Credenciais OAuth (portal.azure.com):**
- Application ID (Client ID)
- Client Secret
- Redirect URI: `http://localhost:6805/oauth/microsoft/callback`

**Endpoints IMAP/SMTP:**
- IMAP: `outlook.office365.com:993` (SSL)
- SMTP: `smtp.office365.com:587` (TLS)

### 3.3 IMAP/SMTP Genérico

**Fluxo de Autenticação:**
```
1. Usuário fornece:
   - Email (ex: user@example.com)
   - Senha (armazenada de forma criptografada)
   - Servidor IMAP (ex: mail.example.com:993)
   - Servidor SMTP (ex: mail.example.com:587)
2. Backend testa conexão IMAP
3. Backend testa envio SMTP
4. Armazena credenciais criptografadas
```

**Suporte:**
- Qualquer servidor IMAP/SMTP padrão
- Comum em emails empresariais auto-hospedados

---

## 4. Arquitetura de Dados

### 4.1 Schema do Database (SQLite)

```sql
-- Contas de email
CREATE TABLE email_accounts (
  id TEXT PRIMARY KEY,
  email TEXT UNIQUE NOT NULL,
  provider TEXT NOT NULL,  -- 'gmail', 'outlook', 'imap'
  display_name TEXT,
  -- Para OAuth
  oauth_access_token TEXT,      -- criptografado
  oauth_refresh_token TEXT,     -- criptografado
  oauth_token_expiry DATETIME,
  -- Para IMAP/SMTP
  imap_host TEXT,
  imap_port INTEGER,
  imap_username TEXT,
  imap_password TEXT,  -- criptografado
  smtp_host TEXT,
  smtp_port INTEGER,
  smtp_username TEXT,
  smtp_password TEXT,  -- criptografado
  sync_enabled BOOLEAN DEFAULT true,
  last_sync DATETIME,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Emails sincronizados
CREATE TABLE emails (
  id TEXT PRIMARY KEY,  -- message_id único
  account_id TEXT NOT NULL REFERENCES email_accounts(id) ON DELETE CASCADE,
  subject TEXT,
  from_address TEXT NOT NULL,
  from_name TEXT,
  to_addresses TEXT,    -- JSON array
  cc_addresses TEXT,    -- JSON array
  bcc_addresses TEXT,   -- JSON array
  body_text TEXT,
  body_html TEXT,
  timestamp DATETIME NOT NULL,
  uid INTEGER,          -- UID IMAP (para sincronização)
  flags INTEGER,        -- flags IMAP (seen, flagged, etc)
  
  -- Para threads
  in_reply_to TEXT,     -- message_id da mensagem original
  references TEXT,      -- JSON array de message_ids
  
  -- Gestão
  is_draft BOOLEAN DEFAULT false,
  folder TEXT,          -- INBOX, SENT, DRAFTS, etc
  labels TEXT,          -- JSON array para Gmail labels
  
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  
  FOREIGN KEY (account_id) REFERENCES email_accounts(id)
);

-- Anexos
CREATE TABLE email_attachments (
  id TEXT PRIMARY KEY,
  email_id TEXT NOT NULL REFERENCES emails(id) ON DELETE CASCADE,
  filename TEXT NOT NULL,
  mime_type TEXT,
  size INTEGER,
  data BLOB,  -- arquivo armazenado localmente
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Drafts (não sincronizados)
CREATE TABLE email_drafts (
  id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL REFERENCES email_accounts(id) ON DELETE CASCADE,
  to_addresses TEXT,    -- JSON array
  cc_addresses TEXT,
  bcc_addresses TEXT,
  subject TEXT,
  body_text TEXT,
  body_html TEXT,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (account_id) REFERENCES email_accounts(id)
);

-- Configurações de sincronização
CREATE TABLE email_settings (
  id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL REFERENCES email_accounts(id) ON DELETE CASCADE,
  sync_interval_minutes INTEGER DEFAULT 5,
  auto_mark_as_read BOOLEAN DEFAULT false,
  keep_local_copy BOOLEAN DEFAULT true,
  max_emails_to_sync INTEGER DEFAULT 1000,
  FOREIGN KEY (account_id) REFERENCES email_accounts(id)
);
```

### 4.2 Estrutura em Memória (Go structs)

```go
type EmailAccount struct {
  ID                 string
  Email              string
  Provider           string  // "gmail", "outlook", "imap"
  DisplayName        string
  OAuthAccessToken   string  // criptografado
  OAuthRefreshToken  string  // criptografado
  IMAPHost           string
  IMAPPort           int
  SMTPHost           string
  SMTPPort           int
  SyncEnabled        bool
  LastSync           time.Time
}

type Email struct {
  ID           string
  AccountID    string
  Subject      string
  From         Person
  To           []Person
  CC           []Person
  BCC          []Person
  BodyText     string
  BodyHTML     string
  Timestamp    time.Time
  UID          uint32        // IMAP UID
  Flags        []string      // ["Seen", "Flagged", etc]
  InReplyTo    string
  References   []string
  IsDraft      bool
  Folder       string
  Labels       []string      // Gmail labels
  Attachments  []Attachment
}

type EmailAttachment struct {
  ID       string
  EmailID  string
  Filename string
  MimeType string
  Size     int64
  Data     []byte
}

type EmailDraft struct {
  ID          string
  AccountID   string
  To          []string
  CC          []string
  BCC         []string
  Subject     string
  BodyText    string
  BodyHTML    string
  CreatedAt   time.Time
  UpdatedAt   time.Time
}
```

---

## 5. Interface do Usuário (Frontend)

### 5.1 Estrutura de Abas

```
┌─────────────────────────────────────────────┐
│ Chat │ Editor │ Channels │ [Emails] │ ⚙️    │
└─────────────────────────────────────────────┘
```

Nova aba "Emails" (semelhante a "Channels" que já existe).

### 5.2 EmailListPage - Layout Principal

```
┌─────────────────────────────────────────────────────┐
│  [◄ Voltar]  Caixa de Entrada                  🔄  │
├──────────────────────────────────────────────────────┤
│ Contas:  [Gmail ▼] [Outlook ▼] [+ Adicionar]       │
│ Pastas:  📥 Entrada  📤 Enviados  📝 Rascunhos      │
│          [Buscar emails...] [Filtros ▼]            │
├──────────────────────────────────────────────────────┤
│ ☐ □ João Silva        Reunião de projeto?         │
│   <joao@example.com>   "Olá, podemos agendar..."   │
│   15:30 hoje                                        │
├──────────────────────────────────────────────────────┤
│ ☐ □ Maria Santos      RE: Documento                │
│   <maria@example.com>  "Segue em anexo o..."       │
│   14:15 hoje          [📎 1 anexo]                 │
├──────────────────────────────────────────────────────┤
│ ☐ □ Newsletter        News: Novo release           │
│   <news@company.com>   "Versão 2.0 disponível"     │
│   13:00 hoje                                        │
├──────────────────────────────────────────────────────┤
│                   [Carregar mais...]                │
└──────────────────────────────────────────────────────┘
```

**Recursos de Acessibilidade:**
- Lista com navegação por setas (↑/↓)
- Enter para abrir email
- N para novo email
- R para responder (dentro do email)
- Suporte a screen reader com ARIA labels
- Modo de alto contraste (tema escuro/claro)

### 5.3 EmailDetailPage - Visualização

```
┌─────────────────────────────────────────────────────┐
│ ◄ Voltar  De: João Silva <joao@example.com>        │
│           Para: você@gmail.com                      │
│           Cc: maria@example.com                     │
│           Data: 15 de março de 2026, 15:30         │
├─────────────────────────────────────────────────────┤
│ Assunto: Reunião de projeto?                        │
├─────────────────────────────────────────────────────┤
│                                                     │
│ Olá,                                                │
│                                                     │
│ Podemos agendar uma reunião para discutir o        │
│ novo projeto? Estou disponível quinta-feira.       │
│                                                     │
│ Abraços,                                            │
│ João                                                │
│                                                     │
├─────────────────────────────────────────────────────┤
│ [📎] arquivo.pdf (245 KB) [Baixar]                 │
│ [📎] imagem.png (1.2 MB) [Baixar]                  │
├─────────────────────────────────────────────────────┤
│  [Responder] [Responder a Todos] [Encaminhar]      │
│  [🚩 Marcar] [🗑️ Deletar] [• • •]                   │
└─────────────────────────────────────────────────────┘
```

### 5.4 EmailComposePage - Compositor

```
┌─────────────────────────────────────────────────────┐
│ ◄ Cancelar                     [Enviar] [Salvar]    │
├─────────────────────────────────────────────────────┤
│ De: você@gmail.com [▼]                              │
│ Para: [_______________________________]             │
│ Cc:  [_______________________________]             │
│ Bcc: [_______________________________]             │
├─────────────────────────────────────────────────────┤
│ Assunto: [_______________________________]          │
├─────────────────────────────────────────────────────┤
│                                                     │
│  [B I U _ Link • ◦ • •]                            │
│                                                     │
│  ┌─────────────────────────────────────────┐       │
│  │                                         │       │
│  │ Escreva sua mensagem aqui...           │       │
│  │                                         │       │
│  │                                         │       │
│  └─────────────────────────────────────────┘       │
│                                                     │
│  [+ Anexar arquivo] [+ Template] [• • •]          │
│                                                     │
│  📎 assinatura.pdf (120 KB) [✕]                   │
│                                                     │
└─────────────────────────────────────────────────────┘
```

**Recursos:**
- Autocomplete para contatos
- Editor WYSIWYG (ou Markdown)
- Salvar como rascunho automaticamente
- Agenda de envio (enviar depois)

### 5.5 EmailSettingsPage - Configurações

```
┌─────────────────────────────────────────────────────┐
│ ◄ Voltar  Configurações de Email                   │
├─────────────────────────────────────────────────────┤
│ CONTAS DE EMAIL                                     │
│                                                     │
│ ✓ você@gmail.com (Gmail) [Configurar] [Remover]   │
│   Status: Sincronizado há 2 minutos                │
│                                                     │
│ ✓ trabalho@outlook.com (Outlook) [Configurar]     │
│   Status: Sincronizado há 15 minutos               │
│                                                     │
│ [+ Adicionar nova conta]                           │
├─────────────────────────────────────────────────────┤
│ SINCRONIZAÇÃO                                       │
│                                                     │
│ Intervalo de sincronização:  [5 minutos ▼]        │
│ ☑ Manter cópia local de emails                     │
│ ☑ Marcar como lido após visualizar                 │
│ Número máximo de emails para sincronizar: [1000]   │
├─────────────────────────────────────────────────────┤
│ NOTIFICAÇÕES                                        │
│                                                     │
│ ☑ Notificar novos emails                           │
│ ☑ Som                                              │
│ ☑ Badge (conta de não-lidos)                       │
└─────────────────────────────────────────────────────┘
```

---

## 6. Backend: Fluxo de Sincronização

### 6.1 SyncEngine - Proceso de Sincronização Periódica

```
┌─────────────────────────────────────────┐
│     SyncEngine.Start() executado         │
│     a cada 5 minutos                    │
└─────────────┬───────────────────────────┘
              │
              ▼
┌─────────────────────────────────────────┐
│ Para cada conta habilitada:             │
│ 1. Conectar ao IMAP                     │
│ 2. Executar IMAP SELECT INBOX           │
│ 3. Buscar UIDs novos/modificados        │
│ 4. Fetch headers para UIDs novos        │
│ 5. Armazenar no DB                      │
│ 6. Atualizar last_sync                  │
│ 7. Desconectar IMAP                     │
└─────────────┬───────────────────────────┘
              │
              ▼
┌─────────────────────────────────────────┐
│ Se novo email recebido:                 │
│ 1. Converter para Email struct          │
│ 2. Extrair informações de contato       │
│ 3. Notificar Frontend (WebSocket)       │
│ 4. Processar em MessageGateway?         │
│    (se email de contato pareado)        │
└─────────────────────────────────────────┘
```

### 6.2 Fluxo de Envio de Email

```
Frontend                      Backend
   │                           │
   ├─ EmailSend (EmailDraft) ──>│
   │                           │
   │                    Create EmailDraft
   │                    Save to DB
   │                           │
   │                    Get SMTP credentials
   │                    Connect to SMTP
   │                           │
   │                    Send message
   │                           │
   │                    Move to Sent folder
   │                    (IMAP APPEND)
   │                           │
   │<─ EmailSent (ID, Timestamp)
   │                           │
```

---

## 7. Integração com Canais Existentes

### 7.1 Email como Canal de Comunicação

Email pode ser tratado como **canal de comunicação** (como Slack):

```go
// Quando novo email de contato pareado chega:
msg := messaging.IncomingMessage{
  SourceChannel: "email",
  ChatID:        accountID,
  SenderID:      fromAddress,
  SenderName:    fromName,
  Text:          emailBodyText,
  Timestamp:     emailTimestamp,
  // Referência ao email original
  MessageID:     emailMessageID,
}

gateway.HandleIncomingMessage(msg)
// → Responde com Assistente
// → Resposta é armazenada como draft/rascunho
```

### 7.2 Configuração por Perfil

Um perfil específico para emails (como `canais-comunicacao.json`):

```json
{
  "name": "email-assistant",
  "description": "Perfil para assistência via email",
  "temperature": 0.3,
  "max_tokens": 2048,
  "enabled_tools": [],
  "enabled_skills": [
    "search",
    "memory"
  ],
  "channel_response_mode": "always_text"
}
```

---

## 8. Segurança

### 8.1 Armazenamento de Credenciais

**Princípio:** Nunca armazenar credenciais em texto plano.

```go
// Usar criptografia simétrica (AES-256-GCM)
// Master key derivada da senha do usuário ou armazenada seguramente

encryptedToken := crypto.Encrypt(oauthAccessToken, masterKey)
// Armazenar `encryptedToken` no DB

// Ao usar:
token := crypto.Decrypt(encryptedToken, masterKey)
```

### 8.2 Tokens OAuth

**Refresh tokens:**
- Armazenados criptografados no DB
- Usados para renovar access_token automaticamente (antes de expirar)
- Access_token é mantido em memória (não persiste)

### 8.3 HTTPS em Callbacks OAuth

```
Production:
- Callback: https://assistente.example.com/oauth/google/callback

Development:
- Local: http://localhost:6805/oauth/google/callback
- Ngrok: https://xxxx.ngrok.io/oauth/google/callback (para testar com apps reais)
```

### 8.4 Permissões Mínimas

- OAuth scopes: apenas Mail.Read, Mail.Send (não acesso a calendário, contatos, etc)
- Armazenamento: emails em SQLite local, não sincronizado na nuvem
- Dados sensíveis: não enviar para LLM (apenas texto relevante para resposta)

---

## 9. Roadmap de Implementação

### Fase 1: Setup Básico de Email (Semana 1-2)

**Objetivo:** Infraestrutura de autenticação e sincronização IMAP.

**Tarefas:**
- [ ] Criar estrutura de pastas (`internal/email/`)
- [ ] Implementar `EmailAccount` struct e DB schema
- [ ] Implementar IMAP client (conectar, sincronizar, desconectar)
- [ ] Implementar SMTP client (enviar, conectar)
- [ ] Criar `EmailStore` (CRUD em SQLite)
- [ ] Implementar `SyncEngine` básico (sincronização a cada 5 min)
- [ ] Testes unitários para IMAP/SMTP

**Saída:**
- Backend pode conectar/sincronizar/enviar email via IMAP/SMTP genérico
- Schema DB criado e testado

---

### Fase 2: Interface de Leitura (Semana 2-3)

**Objetivo:** Frontend para visualizar emails.

**Tarefas:**
- [ ] Criar `EmailListPage.tsx` (lista simples de emails)
- [ ] Criar `EmailDetailPage.tsx` (visualizar email completo)
- [ ] Implementar endpoints Go para listar/buscar emails
- [ ] Implementar paginação e lazy loading
- [ ] Testes de acessibilidade (ARIA, navegação por teclado)

**Saída:**
- Usuário pode visualizar inbox, ler emails completos

---

### Fase 3: Composição e Resposta (Semana 3-4)

**Objetivo:** Enviar e responder emails.

**Tarefas:**
- [ ] Criar `EmailComposePage.tsx` (novo email)
- [ ] Implementar resposta rápida em `EmailDetailPage`
- [ ] Salvar rascunhos automaticamente (autosave)
- [ ] Implementar `EmailDraft` model e persistência
- [ ] Suporte a anexos (upload, download)
- [ ] Testes de envio

**Saída:**
- Usuário pode compor, responder, e enviar emails
- Suporte a rascunhos

---

### Fase 4: OAuth Gmail (Semana 4-5)

**Objetivo:** Autenticação OAuth para Gmail.

**Tarefas:**
- [ ] Implementar `OAuthManager` para Google
- [ ] Criar endpoints OAuth (authorize, callback, refresh)
- [ ] Frontend: `OAuthCallbackHandler.tsx` + popup
- [ ] Testar fluxo completo (autorizar → sincronizar)
- [ ] Armazenar tokens criptografados

**Saída:**
- Usuário pode conectar Gmail via OAuth
- Sincronização automática de emails do Gmail

---

### Fase 5: OAuth Outlook (Semana 5)

**Objetivo:** Autenticação OAuth para Outlook/Microsoft 365.

**Tarefas:**
- [ ] Implementar `OAuthManager` para Microsoft
- [ ] Adicionar endpoints OAuth Microsoft
- [ ] Frontend: Suporte a múltiplos provedores
- [ ] Testar fluxo Outlook

**Saída:**
- Suporte a Gmail + Outlook
- UI para adicionar múltiplas contas

---

### Fase 6: Gerenciamento Avançado (Semana 6-7)

**Objetivo:** Filtros, pastas, notificações, busca.

**Tarefas:**
- [ ] Implementar busca full-text (SQLite FTS)
- [ ] Filtros por remetente, assunto, data, pasta
- [ ] Gerenciar pastas (mover, arquivar, deletar)
- [ ] Notificações desktop/WebSocket de novos emails
- [ ] Badge de contador não-lidos

**Saída:**
- Usuário pode filtrar, buscar, e organizar emails
- Notificações em tempo real

---

### Fase 7: Integração com Contatos/Chat (Semana 7-8)

**Objetivo:** Email como canal de comunicação.

**Tarefas:**
- [ ] Implementar `EmailGateway` (como `SlackAdapter`)
- [ ] Registrar email em `messaging.Gateway`
- [ ] Converter Email → IncomingMessage (pairing code flow)
- [ ] Criar perfil `email-assistant.json`
- [ ] Respostas automáticas (drafts) do Assistente

**Saída:**
- Email integrado com chat e contatos
- Assistente pode responder emails automaticamente (com aprovação)

---

### Fase 8: Polimento e Performance (Semana 8+)

**Objetivo:** UX final, otimizações, testes.

**Tarefas:**
- [ ] Otimizar carregamento de emails grandes
- [ ] Threads de conversa (agrupar respostas)
- [ ] Suporte a assinatura customizável
- [ ] Template de respostas
- [ ] Testes end-to-end com contas reais
- [ ] Documentação completa

**Saída:**
- Sistema de email completo e pronto para produção

---

## 10. Considerações de UX/Acessibilidade

### 10.1 Design Acessível

Similar ao chat atual:
- **Navegação por teclado:** Tab, Shift+Tab, setas, Enter
- **Screen reader support:** ARIA labels, role attributes
- **Alto contraste:** Tema escuro/claro
- **Tamanho de fonte ajustável**
- **Sem dependência de cor para informação** (ícones + texto)

### 10.2 Padrões de Design

```
Email List:
┌─────────────┐
│ ☑ Unread    │  ← Checkbox + visual indicator
│ □ Read      │  ← Status visível ao lado
└─────────────┘

Responsivo:
Desktop:  [Sidebar] + [List] + [Detail]
Tablet:   [List] + [Detail]  (swipe entre abas)
Mobile:   [List] OR [Detail] (toggle)
```

### 10.3 Performance

- **Lazy loading:** Carregar 20 emails por vez
- **Virtualization:** Renderizar apenas emails visíveis
- **Cache:** Manter emails em memória enquanto sincroniza
- **Índices DB:** UID IMAP, timestamp, account_id

---

## 11. Possíveis Desafios e Soluções

| Desafio | Solução |
|---------|---------|
| Sincronização lenta com muitos emails | Sincronizar só últimos 1000, paginar |
| Tokens OAuth expiram | Refresh automático antes de expirar |
| Credenciais IMAP incorretas | Validar conexão no setup, feedback claro |
| Emails grandes (> 10MB) | Limitar tamanho de sincronização, não armazenar blob |
| Múltiplas contas → confusão de contexto | Seletor visual claro, highlight da conta ativa |
| Segurança: credenciais em memória | Usar secure storage do OS (Keychain/Credential Manager) |
| Integração com LLM | Enviar só texto relevante, não anexos/imagens |

---

## 12. Próximos Passos

1. **Fase 1:** Iniciar com IMAP/SMTP genérico (mais simples, válido para qualquer email)
2. **DB Schema:** Criar migrations para SQLite
3. **Frontend:** Começar com EmailListPage básica
4. **Testes:** Setup de testes com contas de exemplo (mailtrap.io)
5. **Documentação:** Criar guias de setup para Gmail/Outlook (similar a SLACK_CHANNEL_SETUP.md)

---

## Apêndice A: Dependências Go Necessárias

```go
// go.mod additions
require (
  github.com/emersion/go-imap v1.2.1
  github.com/emersion/go-smtp v0.18.1
  github.com/emersion/go-message v0.16.0
  golang.org/x/oauth2 v0.15.0
  google.golang.org/api v0.150.0              // Gmail API (optional, for advanced)
  github.com/microsoft/kiota-go v1.0.0        // Microsoft Graph SDK
)
```

---

## Apêndice B: Endpoints Wails Propostos

```go
// app.go / email_service.go

// Accounts
AddEmailAccount(provider, authData) (accountID, error)
RemoveEmailAccount(accountID) error
ListEmailAccounts() ([]EmailAccount, error)

// Emails
GetEmails(accountID, folder, limit, offset) ([]Email, error)
GetEmail(emailID) (Email, error)
SearchEmails(accountID, query) ([]Email, error)
MarkAsRead(emailID) error

// Compose/Send
SaveDraft(draft EmailDraft) (draftID, error)
SendEmail(emailID or draftID) error
GetDrafts(accountID) ([]EmailDraft, error)

// Sync
StartSync()
StopSync()
SyncNow(accountID)

// OAuth
GetOAuthURL(provider) (url, state, error)
HandleOAuthCallback(provider, code, state) (accountID, error)
```

---

## Conclusão

Este plano proporciona uma estrutura clara para implementar um **sistema de email robusto, acessível e integrado** no Assistente. A abordagem faseada permite:

✅ Começar simples (IMAP/SMTP genérico)
✅ Evoluir para provedores principais (Gmail, Outlook)
✅ Integrar com o sistema de contatos e chat existente
✅ Manter acessibilidade em todas as fases
✅ Validar com usuários entre cada fase

**Tempo estimado:** 8-10 semanas para MVP completo (fases 1-5)
