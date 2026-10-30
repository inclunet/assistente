---
title: "Perfis e Delegação"
weight: 9
---

# Perfis e Delegação

Perfis isolam provedores, voz, skills e contexto por finalidade. A delegação autorizada permite que um perfil atue em nome de outro com limites explícitos.

## Perfis

Cada perfil guarda provedor LLM, voz, skills habilitadas e contexto. Troque em **Configurações → Perfis**; o workspace e as conversas seguem o perfil ativo. Perfis são descobríveis via catálogo e podem ser delegados.

Na página de perfis, a lista ocupa a área disponível e a dica de edição fica
no rodapé, sem cobrir as linhas ou seus botões de ações. Use as setas para
navegar pela grade e Enter para abrir o perfil selecionado.

No editor, os parâmetros de geração que o provedor e o modelo já rejeitaram
explicitamente ficam ocultos. Campos ainda desconhecidos continuam disponíveis.
O app reavalia a lista ao trocar o provedor ou o modelo; ocultar um campo não
apaga nem altera o valor salvo no perfil, que volta a aparecer quando a nova
conexão não tem essa restrição.

Para renomear na própria grade, pressione F2 na célula do nome. Enter confirma
a edição uma única vez; Escape descarta a alteração. A exclusão de um perfil
inativo pede confirmação antes de executar. Enquanto essa confirmação estiver
aberta, os comandos da página ficam bloqueados; responder não libera ações
de outro diálogo ou de outro contexto.

As descrições ajudam o assistente a escolher o perfil adequado:

- **Padrão** atende pesquisa, análise, escrita, organização e solicitações gerais ou ainda pouco definidas.
- **Programação** atende implementação, correção de bugs, depuração, refatoração, testes e revisão de software.

O assistente pode consultar a lista de perfis durante uma conversa. Se uma especialização pontual bastar, ele pode propor um subagente sem alterar o perfil principal. Se a mudança for útil para os próximos turnos, pode propor a troca persistente. O aplicativo sempre pede sua autorização antes de usar outro perfil.

## Modelos e vozes de síntese

Na aba de áudio do perfil, escolha a conexão e o modelo de síntese. As sugestões
HTTP vêm do catálogo público e correspondem ao perfil de servidor, formato de
API e ID exato do modelo. Um modelo oferecido por um proxy mantém seu ID
completo; o aplicativo não tenta descobrir sua origem pelo nome.

Quando o catálogo conhece o modo de seleção, escolher o modelo configura se ele
recebe uma voz separada. Para modelos não catalogados, informe o ID manualmente
no seletor e escolha **Modelo e voz separados** ou **Somente modelo (não enviar
voz)**. No segundo caso, a requisição não contém o parâmetro `voice`.

A lista de vozes informa se é completa, parcial ou desconhecida. Listas parciais
ou desconhecidas permitem digitar o ID da voz. A voz já configurada permanece
visível mesmo quando não está na lista. Atualizar o catálogo não apaga nem troca
os valores que você salvou ou está editando. Vozes locais continuam vindo do
navegador ou do Windows.

Abrir os seletores não executa síntese nem consome créditos. O botão de prévia
faz uma chamada de síntese ao provedor configurado e pode ter custo. Se o
provedor não estiver disponível, o aplicativo não escolhe outro automaticamente.

## Ferramentas e catálogo

O perfil padrão usa o catálogo para descobrir ferramentas sob demanda. Se o
catálogo estiver indisponível, o aplicativo não libera automaticamente todas
as ferramentas: o perfil falha fechado e informa ao assistente que a descoberta
dinâmica não está disponível.

Você pode selecionar ferramentas explicitamente no editor de perfis. Nesse
caso, somente as ferramentas escolhidas ficam disponíveis; essa seleção não
restaura o catálogo nem habilita outras ferramentas implicitamente. Desabilitar
ferramentas ou manter uma seleção explicitamente vazia continua resultando em
nenhuma ferramenta, sem aviso de falha do catálogo.

## Delegação

Em conversas, a autorização para usar outro perfil vale somente para a
invocação atual. Jobs usam um contrato separado: cada job `subagent` precisa de
autorização explícita para cada perfil alvo. A lista e a revogação ficam em
**Jobs → Editar → Perfis autorizados para este job**.

Permissões de jobs não ficam no arquivo do perfil nem no JSON/YAML do job, não
são herdadas por cópias e não são concedidas por importação. Se um perfil for
removido, seus grants são revogados; se a configuração do input `profile`
mudar, será preciso autorizar novamente. Templates dinâmicos nunca recebem
permissão curinga: cada slug permitido é confirmado separadamente.

A delegação é explícita, revogável e auditável, sem compartilhar credenciais.
Veja AEP-0101 para o contrato completo.
