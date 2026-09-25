package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
	"assistente/internal/commandjson"
	"assistente/internal/commandportability"
	"assistente/internal/database"
	"assistente/internal/portability"
	commandtool "assistente/internal/tools/command"
	"gorm.io/gorm"
)

func decodeAgentPayload(raw json.RawMessage, destination any) error {
	canonical, err := commandjson.Canonicalize(raw)
	if err != nil || len(canonical) == 0 || canonical[0] != '{' {
		return commandexecution.ErrInvalidRequest
	}
	decoder := json.NewDecoder(bytes.NewReader(canonical))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return commandexecution.ErrInvalidRequest
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return commandexecution.ErrInvalidRequest
	}
	return nil
}

func (b commandAgentTools) exportAgentConfig(ctx context.Context, c *commandAgentCaller, req commandtool.Request, scope commandconfig.Scope) (any, error) {
	var input struct {
		IncludeCredentials bool `json:"includeCredentials"`
	}
	raw := req.Payload
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	if decodeAgentPayload(raw, &input) != nil || input.IncludeCredentials || len(req.Arguments) != 0 || req.ID != "" || req.LayerID != "" || req.CommandID != "" {
		return nil, commandexecution.ErrInvalidRequest
	}
	epoch, err := c.product.epochs.CaptureAuthenticated(ctx, func(check context.Context) (string, string, error) {
		if err := c.revalidate(check); err != nil {
			return "", "", err
		}
		return c.principal.UserID, c.principal.SessionID, nil
	})
	if err != nil {
		return nil, err
	}
	refs, err := b.app.commandDesktopImportReferences(c.product, ctx)
	if err != nil {
		return nil, err
	}
	var layers []commandportability.LayerExport
	err = database.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := c.revalidateTx(ctx, tx); err != nil {
			return err
		}
		transactionRefs := refs
		transactionRefs.CredentialPattern = commandportability.NewCredentialPatternResolver(tx)
		var err error
		layers, err = commandportability.ExportScopeFromStore(ctx, tx, scope, transactionRefs)
		return err
	})
	if err != nil {
		return nil, err
	}
	if len(layers) > 64 {
		return nil, errCommandExportLimit
	}
	result := portability.ExportFile{Version: portability.ExportVersion, ExportedAt: time.Now().UTC(), Resources: portability.ExportResources{CommandLayers: layers}}
	if err := c.product.epochs.Admit(ctx, epoch, c.revalidate, func() error { return nil }); err != nil {
		return nil, err
	}
	return result, nil
}

func (b commandAgentTools) importAgentConfig(ctx context.Context, c *commandAgentCaller, req commandtool.Request, scope commandconfig.Scope) (any, error) {
	var input struct {
		JSONData    string                         `json:"jsonData"`
		Resolutions []portability.ImportResolution `json:"resolutions"`
	}
	if decodeAgentPayload(req.Payload, &input) != nil || len(req.Arguments) != 0 || req.ID != "" || req.LayerID != "" || req.CommandID != "" {
		return nil, commandexecution.ErrInvalidRequest
	}
	request := portability.ImportRequest{JSONData: input.JSONData, Resolutions: input.Resolutions}
	layers, err := portability.ParseCommandImportEnvelope([]byte(input.JSONData))
	if err != nil {
		return nil, err
	}
	options, err := portability.CommandImportOptions(request)
	if err != nil {
		return nil, err
	}
	// O documento pode nomear referências de origem. Só o workspace ativo
	// autorizado pelo host pode ser destino; nunca outro workspace do usuário.
	for _, layer := range layers {
		if scope.WorkspaceID == nil {
			if layer.Scope.Kind != commandportability.GlobalScope {
				return nil, commandexecution.ErrDenied
			}
		} else {
			if layer.Scope.Kind != commandportability.WorkspaceScope {
				return nil, commandexecution.ErrDenied
			}
			target := options.WorkspaceMap[layer.Scope.WorkspaceID]
			if target == "" {
				target = layer.Scope.WorkspaceID
			}
			if target != *scope.WorkspaceID {
				return nil, commandexecution.ErrDenied
			}
		}
	}
	refs, err := b.app.commandDesktopImportReferences(c.product, ctx)
	if err != nil {
		return nil, err
	}
	inputs, err := b.app.commandDesktopMutationInputs(c.product, ctx)
	if err != nil {
		return nil, err
	}
	baseAuthorize := inputs.Authorize
	inputs.Authorize = func(check context.Context, p auth.LocalSessionPrincipal, target commandconfig.Scope, operation commandconfig.Operation) error {
		if p != c.principal || target.UserID != scope.UserID || !sameCommandWorkspace(target.WorkspaceID, scope.WorkspaceID) {
			return commandexecution.ErrDenied
		}
		if err := c.revalidate(check); err != nil {
			return err
		}
		return baseAuthorize(check, p, target, operation)
	}
	baseHook := inputs.OnMutationTx
	inputs.OnMutationTx = func(check context.Context, tx *gorm.DB, diff commandconfig.MutationDiff) error {
		if diff.Scope.UserID != scope.UserID || !sameCommandWorkspace(diff.Scope.WorkspaceID, scope.WorkspaceID) {
			return commandexecution.ErrDenied
		}
		if err := c.revalidateTx(check, tx); err != nil {
			return err
		}
		return baseHook(check, tx, diff)
	}
	applier, err := b.app.newCommandMutationApplierAuthenticated(inputs, c)
	if err != nil {
		return nil, err
	}
	batch, err := applier.ImportEnvelopeBatch(ctx, "", []byte(input.JSONData), options, refs)
	return commandDesktopImportOutcome(batch, layers, err)
}
