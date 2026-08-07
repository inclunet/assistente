package acpregistry

// Documentos de teste no formato publicado pelo registro. Nenhum teste deste
// pacote fala com a rede de verdade: o índice sempre vem de um httptest ou de
// uma dessas constantes.

// digestDeTeste é o SHA-256 do vazio, em maiúsculas — o formato do registro
// aceita as duas caixas.
const digestDeTeste = "E3B0C44298FC1C149AFBF4C8996FB92427AE41E4649B934CA495991B7852B855"

// indiceBom tem um agente de cada tipo de distribuição, com os campos que a
// tela usa.
const indiceBom = `{
  "version": "1.0.0",
  "agents": [
    {
      "id": "codex-acp",
      "name": "Codex",
      "version": "1.1.9",
      "description": "Ponte ACP para o Codex da OpenAI.",
      "repository": "https://github.com/agentclientprotocol/codex-acp",
      "website": "https://agentclientprotocol.com",
      "authors": ["OpenAI", "Zed Industries"],
      "license": "Apache-2.0",
      "icon": "https://cdn.agentclientprotocol.com/registry/icons/codex-acp.svg",
      "distribution": {
        "npx": { "package": "@agentclientprotocol/codex-acp@1.1.9", "args": ["--acp"] }
      }
    },
    {
      "id": "goose",
      "name": "goose",
      "version": "1.12.0",
      "description": "Agente de código da Block.",
      "authors": ["Block"],
      "license": "Apache-2.0",
      "distribution": {
        "binary": {
          "windows-x86_64": {
            "archive": "https://cdn.agentclientprotocol.com/goose/goose-windows.zip",
            "sha256": "E3B0C44298FC1C149AFBF4C8996FB92427AE41E4649B934CA495991B7852B855",
            "cmd": "./goose-package\\goose.exe",
            "args": ["acp"],
            "env": { "GOOSE_ACP": "1" }
          }
        }
      }
    },
    {
      "id": "fast-agent",
      "name": "fast-agent",
      "version": "0.4.2",
      "description": "Agente distribuído pelo uv.",
      "license": "Apache-2.0",
      "distribution": {
        "uvx": { "package": "fast-agent-mcp", "args": ["acp"] }
      }
    }
  ]
}`

// indiceComOutroAgente serve para ver a revalidação trocando o conteúdo.
const indiceComOutroAgente = `{
  "version": "1.0.0",
  "agents": [
    {
      "id": "kimi",
      "name": "Kimi CLI",
      "version": "2.0.0",
      "description": "Chegou na revalidação.",
      "distribution": { "npx": { "package": "kimi-cli", "args": ["--acp"] } }
    }
  ]
}`

// indiceComTextoMalicioso é o que o D9 existe para tratar: escape de terminal,
// caractere de controle, marca de inversão de direção e URL que não é https.
const indiceComTextoMalicioso = `{
  "version": "1.0.0",
  "agents": [
    {
      "id": "sujo",
      "name": "\u001b[31mAgente\u001b[0m",
      "version": "1.0.0\u0007",
      "description": "primeira linha\nsegunda linha\u202Eexe.pngo",
      "repository": "https://exemplo.test/repo",
      "website": "javascript:alert(1)",
      "icon": "http://exemplo.test/icone.svg",
      "authors": ["A\u0008utor", "   "],
      "license": "MIT\u001b]0;titulo\u0007",
      "distribution": {
        "npx": { "package": "@escopo/pacote@1.0.0", "args": ["--acp\u001b[0m"] }
      }
    }
  ]
}`

// indiceMalformado é JSON válido cuja lista de agentes não é lista.
const indiceMalformado = `{"version": "1.0.0", "agents": {"codex-acp": {}}}`

// indiceDeMajorDesconhecido troca o contrato do documento.
const indiceDeMajorDesconhecido = `{
  "version": "2.0.0",
  "agents": [
    { "id": "codex-acp", "name": "Codex", "distribution": { "npx": { "package": "p" } } }
  ]
}`

// indiceSemAgentes é estruturalmente válido e não é catálogo.
const indiceSemAgentes = `{"version": "1.0.0", "agents": []}`
