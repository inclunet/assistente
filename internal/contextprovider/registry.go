package contextprovider

import (
	"context"
	"log"
	"sort"
	"strings"
)

type Registry struct {
	providers []Provider
}

func NewRegistry(providers ...Provider) *Registry {
	r := &Registry{}
	for _, provider := range providers {
		r.Register(provider)
	}
	return r
}

func (r *Registry) Register(provider Provider) {
	if provider == nil {
		return
	}
	r.providers = append(r.providers, provider)
	sort.SliceStable(r.providers, func(i, j int) bool {
		return r.providers[i].Name() < r.providers[j].Name()
	})
}

func (r *Registry) Providers() []Provider {
	if r == nil || len(r.providers) == 0 {
		return nil
	}
	return append([]Provider(nil), r.providers...)
}

func (r *Registry) Metadata() []ProviderMetadata {
	if r == nil || len(r.providers) == 0 {
		return []ProviderMetadata{}
	}
	items := make([]ProviderMetadata, 0, len(r.providers))
	for _, provider := range r.providers {
		if metadataProvider, ok := provider.(MetadataProvider); ok {
			metadata := metadataProvider.Metadata()
			if metadata.Name == "" {
				metadata.Name = provider.Name()
			}
			items = append(items, metadata)
			continue
		}
		items = append(items, ProviderMetadata{
			Name:           provider.Name(),
			DisplayName:    provider.Name(),
			Description:    "",
			DefaultEnabled: true,
		})
	}
	return items
}

func (r *Registry) Build(ctx context.Context, req BuildRequest) ([]Block, error) {
	if r == nil {
		return nil, nil
	}
	var blocks []Block
	for _, provider := range r.providers {
		if !req.Enabled(provider.Name()) {
			continue
		}
		providerBlocks, err := provider.Build(ctx, req)
		blocks = appendProviderBlocks(blocks, provider.Name(), providerBlocks)
		if err != nil {
			log.Printf("[context/providers] provider %q ignorado após erro: %v", provider.Name(), err)
			continue
		}
	}
	sortBlocks(blocks)
	return blocks, nil
}

func appendProviderBlocks(blocks []Block, providerName string, providerBlocks []Block) []Block {
	for _, block := range providerBlocks {
		if strings.TrimSpace(block.Content) == "" {
			continue
		}
		if block.Provider == "" {
			block.Provider = providerName
		}
		blocks = append(blocks, block)
	}
	return blocks
}

func RenderBlocks(blocks []Block) []string {
	out := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if trimmed := strings.TrimSpace(block.Content); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func sortBlocks(blocks []Block) {
	sort.SliceStable(blocks, func(i, j int) bool {
		if blocks[i].Volatility != blocks[j].Volatility {
			return volatilityRank(blocks[i].Volatility) < volatilityRank(blocks[j].Volatility)
		}
		if blocks[i].Priority != blocks[j].Priority {
			return blocks[i].Priority < blocks[j].Priority
		}
		if blocks[i].Provider != blocks[j].Provider {
			return blocks[i].Provider < blocks[j].Provider
		}
		return blocks[i].Name < blocks[j].Name
	})
}

func volatilityRank(value Volatility) int {
	switch value {
	case VolatilityStable:
		return 0
	case VolatilityLowDynamic:
		return 1
	case VolatilitySlowDynamic:
		return 2
	case VolatilityMidDynamic:
		return 3
	case VolatilityRolling:
		return 4
	case VolatilityFastDynamic:
		return 5
	case VolatilityTurnDynamic:
		return 6
	default:
		return 9
	}
}
