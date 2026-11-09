package app

import (
	"testing"

	"assistente/internal/configdir"
)

// Contratos de mapa e hot-swap usam o produto real, sem preparar manutenção
// de jobs nem reiniciar um App antes de iniciar o cenário. As provas de
// manutenção/restart e do worker que regenera jobs mantêm suas fixtures.
func commandCatalogProductFixture(t *testing.T) *App {
	t.Helper()
	root := t.TempDir()
	t.Cleanup(configdir.ResetForTests)
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	configdir.ResetForTests()
	return readyCommandProduct(t)
}
