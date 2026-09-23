package app

import (
	"context"
	"encoding/base64"
	"encoding/json"

	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
	"assistente/internal/commandimage"
	"gorm.io/gorm"
)

// image_upload is a transient UI transport field, never a configuration field.
// Normalize before canonicalization/confirmation: the receipt binds the digest,
// while the bytes remain private to this request until its transaction commits.
func prepareCommandSettingsImage(req CommandSettingsMutationRequest) (CommandSettingsMutationRequest, *commandimage.Asset, error) {
	if req.Binding == nil {
		return req, nil, nil
	}
	upload, exists := req.Binding.Presentation["image_upload"]
	if !exists {
		return req, nil, nil
	}
	if req.Operation != string(commandconfig.BindingCreate) && req.Operation != string(commandconfig.BindingUpdate) {
		return req, nil, commandexecution.ErrInvalidRequest
	}
	encoded, ok := upload.(string)
	if !ok || len(encoded) == 0 || len(encoded) > base64.StdEncoding.EncodedLen(1<<20) {
		return req, nil, commandexecution.ErrInvalidRequest
	}
	if _, exists := req.Binding.Presentation["image_ref"]; exists {
		return req, nil, commandexecution.ErrInvalidRequest
	}
	data, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return req, nil, commandexecution.ErrInvalidRequest
	}
	asset, err := commandimage.Normalize(data)
	if err != nil {
		return req, nil, commandexecution.ErrInvalidRequest
	}
	binding := *req.Binding
	binding.Presentation = make(map[string]any, len(req.Binding.Presentation))
	for key, value := range req.Binding.Presentation {
		if key != "image_upload" {
			binding.Presentation[key] = value
		}
	}
	binding.Presentation["image_ref"] = asset.Ref
	req.Binding = &binding
	return req, &asset, nil
}

func commitCommandSettingsImage(ctx context.Context, tx *gorm.DB, userID string, intent commandconfig.MutationIntent, asset *commandimage.Asset) error {
	if intent.Binding == nil {
		return nil
	}
	var presentation struct {
		ImageRef string `json:"image_ref"`
	}
	if err := json.Unmarshal([]byte(intent.Binding.Presentation), &presentation); err != nil {
		return commandexecution.ErrInvalidRequest
	}
	if asset != nil {
		if presentation.ImageRef != asset.Ref {
			return commandexecution.ErrInvalidRequest
		}
		if err := commandimage.PruneTx(ctx, tx, userID); err != nil {
			return err
		}
		if err := commandimage.PutTx(ctx, tx, userID, *asset); err != nil {
			return err
		}
	}
	if presentation.ImageRef != "" {
		if _, err := commandimage.Load(ctx, tx, userID, presentation.ImageRef); err != nil {
			return commandexecution.ErrInvalidRequest
		}
	}
	return nil
}
