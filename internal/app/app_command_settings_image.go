package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"

	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
	"assistente/internal/commandimage"
	"gorm.io/gorm"
)

const (
	maxCommandSettingsImageUploads = 11 // imagem base e até dez estados
	maxCommandSettingsImageBytes   = maxCommandSettingsImageUploads * (1 << 20)
)

type commandSettingsImageBatch struct {
	assets []commandimage.Asset
}

// image_upload is a transient UI transport field, never a configuration field.
// Normalize before canonicalization/confirmation: the receipt binds the digest,
// while the bytes remain private to this request until its transaction commits.
func prepareCommandSettingsImage(req CommandSettingsMutationRequest) (CommandSettingsMutationRequest, *commandSettingsImageBatch, error) {
	if req.Binding == nil {
		return req, nil, nil
	}
	if _, exists := req.Binding.Presentation["image_upload"]; !exists {
		states, hasStates := req.Binding.Presentation["states"]
		if !hasStates {
			return req, nil, nil
		}
		stateMap, ok := states.(map[string]any)
		if !ok {
			return req, nil, nil
		}
		hasUpload := false
		for _, stateValue := range stateMap {
			if state, ok := stateValue.(map[string]any); ok {
				if _, found := state["image_upload"]; found {
					hasUpload = true
					break
				}
			}
		}
		if !hasUpload {
			return req, nil, nil
		}
	}
	if req.Operation != string(commandconfig.BindingCreate) && req.Operation != string(commandconfig.BindingUpdate) {
		return req, nil, commandexecution.ErrInvalidRequest
	}
	binding := *req.Binding
	binding.Presentation = make(map[string]any, len(req.Binding.Presentation))
	for key, value := range req.Binding.Presentation {
		binding.Presentation[key] = value
	}
	batch := &commandSettingsImageBatch{}
	seen := make(map[string]struct{})
	totalBytes := 0
	uploadCount := 0
	process := func(fields map[string]any) error {
		upload, hasUpload := fields["image_upload"]
		if !hasUpload {
			return nil
		}
		if _, hasRef := fields["image_ref"]; hasRef {
			return commandexecution.ErrInvalidRequest
		}
		encoded, ok := upload.(string)
		if !ok || len(encoded) == 0 || len(encoded) > base64.StdEncoding.EncodedLen(1<<20) || uploadCount >= maxCommandSettingsImageUploads {
			return commandexecution.ErrInvalidRequest
		}
		uploadCount++
		data, err := base64.StdEncoding.Strict().DecodeString(encoded)
		if err != nil || len(data) > 1<<20 || totalBytes > maxCommandSettingsImageBytes-len(data) {
			return commandexecution.ErrInvalidRequest
		}
		totalBytes += len(data)
		asset, err := commandimage.Normalize(data)
		if err != nil {
			return commandexecution.ErrInvalidRequest
		}
		delete(fields, "image_upload")
		fields["image_ref"] = asset.Ref
		if _, duplicate := seen[asset.Ref]; !duplicate {
			batch.assets = append(batch.assets, asset)
			seen[asset.Ref] = struct{}{}
		}
		return nil
	}
	if err := process(binding.Presentation); err != nil {
		return req, nil, err
	}
	if rawStates, ok := binding.Presentation["states"]; ok {
		states, ok := rawStates.(map[string]any)
		if ok {
			clonedStates := make(map[string]any, len(states))
			for name, rawState := range states {
				state, ok := rawState.(map[string]any)
				if !ok {
					clonedStates[name] = rawState
					continue
				}
				cloned := make(map[string]any, len(state))
				for key, value := range state {
					cloned[key] = value
				}
				if err := process(cloned); err != nil {
					return req, nil, err
				}
				clonedStates[name] = cloned
			}
			binding.Presentation["states"] = clonedStates
		}
	}
	req.Binding = &binding
	return req, batch, nil
}

func commitCommandSettingsImage(ctx context.Context, tx *gorm.DB, userID string, intent commandconfig.MutationIntent, batch *commandSettingsImageBatch, before []commandconfig.Binding) error {
	if intent.Binding == nil {
		return nil
	}
	refs, err := commandSettingsImageReferences(intent.Binding.Presentation)
	if err != nil {
		return err
	}
	var retained map[string]struct{}
	if intent.Operation == commandconfig.BindingUpdate && intent.ID != "" {
		for _, binding := range before {
			if binding.ID == intent.ID && binding.UserID == userID {
				retained, err = commandSettingsImageReferences(binding.Presentation)
				if err != nil {
					return err
				}
				break
			}
		}
	}
	// The confirmed binding has already been written in this transaction.
	// Reclaim its replaced assets before checking quota for the new batch;
	// any later failure rolls back both the binding and this pruning.
	if err := commandimage.PruneTx(ctx, tx, userID); err != nil {
		return err
	}
	if batch != nil {
		for _, asset := range batch.assets {
			if _, referenced := refs[asset.Ref]; !referenced {
				return commandexecution.ErrInvalidRequest
			}
			if err := commandimage.PutTx(ctx, tx, userID, asset); err != nil {
				return err
			}
		}
	}
	for ref := range refs {
		if _, err := commandimage.Load(ctx, tx, userID, ref); err != nil {
			// A portable configuration may already reference an unavailable
			// asset. Preserve that same binding's reference on unrelated edits,
			// without granting access to any other owner's bytes or accepting
			// a newly introduced reference without an owned asset.
			if _, unchanged := retained[ref]; unchanged && errors.Is(err, commandimage.ErrNotFound) {
				continue
			}
			return commandexecution.ErrInvalidRequest
		}
	}
	return commandimage.PruneTx(ctx, tx, userID)
}

func commandSettingsImageReferences(raw string) (map[string]struct{}, error) {
	var presentation map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &presentation); err != nil || presentation == nil {
		return nil, commandexecution.ErrInvalidRequest
	}
	refs := make(map[string]struct{})
	collectRef := func(fields map[string]json.RawMessage) error {
		if _, found := fields["image_upload"]; found {
			return commandexecution.ErrInvalidRequest
		}
		if rawRef, found := fields["image_ref"]; found {
			var ref string
			if err := json.Unmarshal(rawRef, &ref); err != nil || !commandimage.ValidRef(ref) {
				return commandexecution.ErrInvalidRequest
			}
			refs[ref] = struct{}{}
		}
		return nil
	}
	if err := collectRef(presentation); err != nil {
		return nil, err
	}
	if rawStates, found := presentation["states"]; found {
		var states map[string]json.RawMessage
		if err := json.Unmarshal(rawStates, &states); err != nil || states == nil {
			return nil, commandexecution.ErrInvalidRequest
		}
		for _, rawState := range states {
			var state map[string]json.RawMessage
			if err := json.Unmarshal(rawState, &state); err != nil || state == nil {
				return nil, commandexecution.ErrInvalidRequest
			}
			if err := collectRef(state); err != nil {
				return nil, err
			}
		}
	}
	return refs, nil
}
