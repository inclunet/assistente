package tools

// protectedBaseTools é o conjunto base protegido de tools: control-plane
// (descoberta/carregamento de capabilities) + base de runtime que o agente usa
// para operar independentemente do domínio da tarefa. A allowlist (tools.allowed)
// de uma skill carregada via load_skill faz *narrowing* das tools de domínio, mas
// NUNCA pode remover implicitamente estas tools — caso contrário a skill amputaria
// o runtime (perderia a capacidade de descobrir/carregar outras tools, registrar
// memória, planejar/anotar tarefas ou reler resultados truncados) só por declarar
// uma allowlist focada no seu próprio domínio.
//
// Este conjunto é isento apenas do narrowing IMPLÍCITO da allowlist. Ele NÃO
// sobrepõe:
//   - deny explícito da skill (tools.denied): o dono da skill que nega uma base
//     por nome continua sendo respeitado;
//   - o estado disabled do perfil (AEP-0081 D2): se o perfil desliga a tool, ela
//     permanece indisponível, base ou não.
//
// Fonte única de verdade compartilhada pelo gate de execução
// (validateExecutionContextToolAccess) e pela seleção de tools anunciadas
// (chat.applySkillScope), para não divergirem prompt↔defs↔execução.
//
// Os nomes vêm das constantes do pacote quando existem (tool_catalog, load_skill).
// memory/task/task_list/task_note/update_plan/read_tool_result são definidos em
// subpacotes que importam este pacote; usá-los como constante criaria ciclo de
// import, então ficam como literais — todos são tools efetivamente registradas.
var protectedBaseTools = map[string]struct{}{
	ToolCatalogName:    {}, // control-plane: descoberta de tools
	LoadSkillName:      {}, // control-plane: carregamento de skills
	"memory":           {}, // base de runtime: memória do agente
	"task":             {}, // base de runtime: gestão de tarefas
	"task_list":        {}, // base de runtime: gestão de tarefas
	"task_note":        {}, // base de runtime: gestão de tarefas
	"update_plan":      {}, // base de runtime: planejamento do turno
	"read_tool_result": {}, // base de runtime: releitura de resultados truncados
}

// IsProtectedBaseTool informa se a tool pertence ao conjunto base protegido, que
// a allowlist de uma skill não pode remover implicitamente. Ver protectedBaseTools
// para a lista curada e a justificativa.
func IsProtectedBaseTool(name string) bool {
	_, ok := protectedBaseTools[name]
	return ok
}
