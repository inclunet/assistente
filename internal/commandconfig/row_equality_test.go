package commandconfig

import (
	"testing"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
)

func TestPersistedRowsCompareInstantsWithoutIgnoringChanges(t *testing.T) {
	instant := time.Date(2026, 11, 8, 3, 0, 0, 123456789, time.UTC)
	for _, zone := range []*time.Location{time.FixedZone("", 0), time.FixedZone("local", -3*3600)} {
		same := instant.In(zone)
		later := same.Add(time.Nanosecond)
		t.Run(zone.String(), func(t *testing.T) {
			layer := Layer{ID: "layer", UserID: "owner", CreatedAt: instant, UpdatedAt: instant}
			copy := layer
			copy.CreatedAt, copy.UpdatedAt = same, same
			if !equalRows([]Layer{layer}, []Layer{copy}) || !sameLayerRows([]Layer{layer}, []Layer{copy}) {
				t.Fatal("mesmo instante recusado")
			}
			if copy.UpdatedAt.Location() != zone {
				t.Fatal("comparação alterou entrada")
			}
			copy.UpdatedAt = later
			if equalRow(layer, copy) {
				t.Fatal("mudança de nanossegundo ignorada")
			}
			copy.UpdatedAt, copy.UserID = same, "other"
			if equalRow(layer, copy) {
				t.Fatal("mudança de owner ignorada")
			}

			generation := Generation{ID: "generation", Generation: 2, UpdatedAt: instant}
			otherGeneration := generation
			otherGeneration.UpdatedAt = same
			if !equalRow(generation, otherGeneration) {
				t.Fatal("mesma geração recusada")
			}
			otherGeneration.Generation++
			if equalRow(generation, otherGeneration) {
				t.Fatal("geração diferente ignorada")
			}

			grant := commandautomation.Grant{ID: "grant", GrantedAt: instant, RevokedAt: &instant}
			otherGrant := grant
			otherGrant.GrantedAt, otherGrant.RevokedAt = same, &same
			if !equalRow(grant, otherGrant) {
				t.Fatal("mesma concessão recusada")
			}
			otherGrant.RevokedAt = &later
			if equalRow(grant, otherGrant) {
				t.Fatal("revogação diferente ignorada")
			}
			otherGrant.RevokedAt = nil
			if equalRow(grant, otherGrant) {
				t.Fatal("revogação ausente ignorada")
			}

			claim := commandactivation.Claim{ActivationID: "claim", ActivatedAt: instant, UpdatedAt: instant, ExpiresAt: &instant, SourceReplayDeadline: &instant}
			otherClaim := claim
			otherClaim.ActivatedAt, otherClaim.UpdatedAt = same, same
			otherClaim.ExpiresAt, otherClaim.SourceReplayDeadline = &same, &same
			if !equalRow(claim, otherClaim) {
				t.Fatal("mesma claim recusada")
			}
			otherClaim.SourceReplayDeadline = &later
			if equalRow(claim, otherClaim) {
				t.Fatal("prazo de replay diferente ignorado")
			}
			otherClaim.SourceReplayDeadline, otherClaim.ExpiresAt = &same, &later
			if equalRow(claim, otherClaim) {
				t.Fatal("expiração diferente ignorada")
			}
		})
	}
}
