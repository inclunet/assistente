---
title: "Atualizações e compatibilidade"
weight: 25
---

# Atualizações e compatibilidade

Você pode atualizar diretamente de qualquer versão publicada do Assistente
para a versão mais recente. Não é necessário instalar versões intermediárias.

## Antes de atualizar

1. Feche o Assistente.
2. Faça uma cópia da pasta de dados do aplicativo.
3. Instale ou extraia a versão nova sobre a instalação conforme o formato que
   você usa.
4. Abra o aplicativo e aguarde a conclusão da inicialização.

O primeiro início pode levar mais tempo porque o banco e configurações antigas
são convertidos. As conversões são idempotentes: uma interrupção segura pode
ser retomada na próxima abertura. Arquivos antigos de canais, contatos, jobs,
skills e MCP continuam disponíveis como fonte de importação; o Assistente não
os apaga automaticamente.

## Diagnóstico local

O log de inicialização inclui uma linha `UpgradeDiagnostic` com:

- versão atual do schema;
- versão mais recente conhecida pelo executável;
- quantidade de migrações aplicadas;
- números das migrações ainda pendentes.

Esse diagnóstico não contém conteúdo de conversas, credenciais, caminhos,
nomes, IDs de usuário nem outros dados pessoais. Em caso de falha, anexe essa
linha e a mensagem de erro ao relato, mas não envie o banco de dados.

## Backups exportados

Arquivos de conversas exportados pela versão 0.1.9 e arquivos portáteis
`version: 2` das versões posteriores podem ser importados diretamente pela
versão atual. O adaptador de 0.1.9 converte IDs numéricos em IDs estáveis e
preserva relações entre mensagens.

## Se a atualização falhar

Não apague os arquivos legados nem use a limpeza de canais antes de confirmar
que os recursos foram importados. Restaure a cópia da pasta de dados e abra
uma issue informando a versão de origem, a versão de destino e o diagnóstico
local sem PII.
