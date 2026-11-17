package wailsapi

import (
	"context"
	"errors"
	"sync"

	"assistente/controllers"
	"assistente/internal/authorization"
)

type Permissions struct {
	mu         sync.RWMutex
	session    Session
	controller *controllers.PermissionsController
}

func NewPermissions() *Permissions { return &Permissions{} }
func AttachPermissions(api *Permissions, session Session, controller *controllers.PermissionsController) {
	if api == nil {
		return
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	api.session = session
	api.controller = controller
}
func (api *Permissions) deps() (Session, *controllers.PermissionsController, error) {
	api.mu.RLock()
	defer api.mu.RUnlock()
	if api.session == nil || api.controller == nil {
		return nil, nil, errors.New("permissions_not_ready")
	}
	return api.session, api.controller, nil
}
func (api *Permissions) GetPermissions() (controllers.PermissionSnapshot, error) {
	session, controller, err := api.deps()
	if err != nil {
		return controllers.PermissionSnapshot{}, err
	}
	return WithUser(session, controller.List)
}
func (api *Permissions) RevokePermission(id string) error {
	session, controller, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) { return struct{}{}, controller.Revoke(ctx, id) })
	return err
}

type PendingPermissionOrigin struct {
	ID     string `json:"id"`
	Domain string `json:"domain"`
	Source string `json:"source"`
}
type PermissionMigration struct {
	Origins []PendingPermissionOrigin         `json:"origins"`
	Owners  []authorization.LegacyOwnerOption `json:"owners"`
}

func (api *Permissions) GetPermissionMigration() (PermissionMigration, error) {
	session, controller, err := api.deps()
	if err != nil {
		return PermissionMigration{}, err
	}
	return WithAdmin(session, func(ctx context.Context) (PermissionMigration, error) {
		result := PermissionMigration{Origins: []PendingPermissionOrigin{}}
		rows, err := controller.Repository.PendingOrigins(ctx)
		if err != nil {
			return result, err
		}
		for _, row := range rows {
			result.Origins = append(result.Origins, PendingPermissionOrigin{ID: row.ID, Domain: row.Domain, Source: row.Source})
		}
		result.Owners, err = controller.Repository.LegacyOwners(ctx)
		return result, err
	})
}
func (api *Permissions) AssignPermissionOrigin(id, userID string) error {
	session, controller, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithAdmin(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, controller.Repository.AssignOrigin(ctx, id, userID)
	})
	return err
}
