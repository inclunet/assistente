---
title: "Fontes de credenciais"
---

# Fontes de credenciais

Em **Credenciais → Nova**, escolha a fonte e depois o tipo de autenticação.
O tipo define como o segredo é aplicado: Bearer, Basic ou header customizado.
O tipo Segredo atende consumidores internos, como canais.

- **Valor salvo:** informe o segredo no campo oculto.
- **Variável de ambiente:** escolha ou digite o nome da variável, sem prefixo.
  O app lê o ambiente herdado no início do processo; reinicie após mudanças externas.
- **Keyring:** escolha um target do Credential Manager do Windows, ou preencha
  serviço e usuário do keyring do sistema. Use apenas uma das alternativas.
  Listagem de targets depende da plataforma; entrada manual continua disponível.
- **Comando:** informe executável, argumentos como array JSON de strings e timeout
  entre 1 e 300 segundos. O padrão é 30 segundos. Não há shell implícito:
  pipes, redirecionamentos e expansões não são interpretados.
- **OAuth:** reservado para uma evolução futura; ainda não pode ser salvo como
  fonte. Isso não altera o OAuth dos servidores MCP.

Exemplo local: executável `nu`, argumentos `["genai", "ai-gateway", "token"]`.
No Windows via WSL: executável `wsl.exe`, argumentos
`["--", "nu", "genai", "ai-gateway", "token"]`.

O comando precisa estar instalado e autenticado, retornar um token em stdout
(em uma linha, até 64 KiB) e terminar com sucesso. Espaços externos são removidos.
O Assistente não exibe stdout nem stderr em erros. A execução ocorre com as
permissões do app; evite segredos literais nos argumentos. Timeout cancela o
processo direto, sem promessa de encerrar todos os descendentes ou processos WSL.
Para **Comando**, o resultado é reutilizado em memória, cifrado, separado por
credencial e usuário. Não há TTL presumido, leitura de JWT ou renovação periódica:
a saída é tratada como token opaco. Chamadas simultâneas aguardam a mesma execução
bem-sucedida; cancelar uma espera não cancela a execução de outra chamada.

Nas requisições HTTP que usam o transport de credenciais (incluindo provedores
LLM), uma resposta **401** descarta o token rejeitado. Se o corpo puder ser
recriado, o Assistente obtém um token novo e repete a requisição **no máximo uma
vez**, apenas se o valor mudou. Se o comando devolver o mesmo valor, chamadas
concorrentes compartilham esse resultado; uma nova rejeição pode tentar renovar. Uploads sem corpo repetível não são reenviados;
a próxima chamada obtém um novo token. Erros 400, 403, de rede ou de banco não
renovam credenciais. Se o servidor indicar expiração de outra forma, salve
novamente a configuração da credencial para limpar o cache.

Salvar/excluir a credencial, recarregar uma configuração alterada, mudar/encerrar a sessão
ou fechar o app descarta o cache. Renovar o login do mesmo usuário ou recarregar
uma configuração idêntica preserva o cache. O token materializado não vai para o banco nem
para a tela de edição. Env e keyring continuam sendo consultados a cada uso.
Consumidores que obtêm o segredo diretamente, sem esse transport HTTP (como
ferramentas HTTP e o SDK Gemini), continuam executando o comando a cada uso.
Sondagens e clientes de provedores LLM (incluindo a listagem de modelos do
formulário e a sondagem do assistente de configuração) não seguem
redirecionamentos para outra origem. Se o comando falhar, o monitor informa
erro de credencial, em vez de indicar que a URL está inacessível.
As sondagens de conexão e o monitor de saúde do provedor compartilham o cache
do transport; chaves digitadas apenas para teste continuam isoladas.

Em Basic, o usuário fica na configuração e a fonte fornece a senha. Em header
customizado, informe o nome do header; a fonte fornece seu valor.

## Provedores LLM

Cadastre a credencial com o hostname da URL base como padrão (por exemplo,
`gateway.example.com`). Ao criar o provedor, marque **Usar credencial já
cadastrada para este domínio**. Na edição, mantenha a chave vazia para preservar
a fonte. Teste de conexão, modelos e execução usam a credencial resolvida.
Digitar uma nova chave e salvar substitui a fonte por **Valor salvo**.
Testar uma chave digitada não substitui nem remove a credencial salva.

## Reconfiguração após atualização

Credenciais antigas sem source precisam ser reconfiguradas manualmente.
Não existe migração automática. `env://NOME` e `keyring://entrada` não são
referências suportadas; escolha a fonte correspondente e informe seus campos.
Esses textos, se gravados em Valor salvo, são tratados literalmente.

A política de redirects fica também nos construtores HTTP de credenciais
compartilhados por chat, TTS e Whisper: o destino deve manter esquema, host e porta.


## Diagnóstico das execuções por comando

O log padrão (`assistente.log`) registra `component=credentials.command` e
`msg=credential_command_execution` uma vez ao terminar cada execução real.
Reutilizar um token em cache não gera essa linha. Não é necessário habilitar
logs detalhados nem reiniciar o app com uma flag.

- `reason=initial`: primeira resolução da entrada em cache, inclusive após edição ou troca de sessão.
- `reason=http_401`: execução para renovar após rejeição HTTP 401. Chamadas concorrentes que compartilham a renovação não multiplicam esse registro.
- `reason=direct`: consumidor sem cache, como resolução direta da fonte.
- `outcome=success|failure|timeout|canceled`: resultado da execução/validação da saída; sucesso não confirma que o servidor aceitou o token novo.
- `duration_ms`: duração do comando e validação da saída, sem o tempo de espera pelo cache ou pelo servidor HTTP.
- `credential_id`: ID persistido, quando disponível; `cache_ref`: identificador opaco da entrada em memória; `cache_generation`: geração invalidada por 401. A referência muda ao substituir a entrada ou reiniciar o app. Consumidores diretos podem não ter esses identificadores.

O contexto disponível conserva IDs de usuário, conversa, perfil e job para
correlação. Token, comando, argumentos, variáveis de ambiente, stdout e stderr
não são registrados. Falha ao iniciar o executável também conta como tentativa;
validação recusada antes de chegar à execução não conta. Um processo ainda em
execução ou interrompido pelo encerramento abrupto do app pode não ter linha final.

Para contar tentativas de renovação presentes no arquivo, no PowerShell:

```powershell
(Get-Content -LiteralPath .\assistente.log | Where-Object {
    $_ -match 'credential_command_execution' -and $_ -match 'reason=http_401'
} | Measure-Object).Count
```

Acrescente `-and $_ -match 'outcome=success'` para contar somente as execuções
bem-sucedidas. O total cobre apenas o histórico disponível no arquivo, não um
contador persistente. Para investigar uma credencial, filtre também seu
`credential_id` ou `cache_ref`.
