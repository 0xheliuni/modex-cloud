// Package validate enforces the per-supplier authorization and whitelist rules
// before a channel is accepted. It is pure logic over already-loaded models so
// it is easy to unit-test.
package validate

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/modex/agt-vault/common"
	"github.com/modex/agt-vault/constant"
	"github.com/modex/agt-vault/model"
)

// ChannelInput is the validated, normalized form of a supplier's upload request.
type ChannelInput struct {
	PlatformId int
	Type       int
	Models     []string
	Group      string
	BaseURL    string
}

// Result carries the normalized channel fields after a successful check.
type Result struct {
	ModelsCSV string
	Group     string
	BaseURL   string
}

// Effective whitelist = platform whitelist, optionally narrowed by the grant.
type effective struct {
	types                                                     map[int]bool
	models                                                    map[string]bool
	groups                                                    map[string]bool
	bases                                                     []string // allowed base-URL prefixes; empty => any
	typesUnrestricted, modelsUnrestricted, groupsUnrestricted bool
}

// ChannelUpload validates an upload against the platform + grant, returning the
// normalized fields to persist, or a descriptive error naming the first
// violation. The grant must already be confirmed to exist (authorization check).
func ChannelUpload(in ChannelInput, platform *model.Platform, grant *model.Grant) (*Result, error) {
	if !constant.IsSupportedChannelType(in.Type) {
		return nil, fmt.Errorf("channel type %d is not supported", in.Type)
	}

	eff := computeEffective(platform, grant)

	// Type.
	if !eff.typesUnrestricted && !eff.types[in.Type] {
		return nil, fmt.Errorf("channel type %d (%s) is not permitted on this platform",
			in.Type, constant.SupportedChannelTypes[in.Type])
	}

	// Models.
	models := normalizeModels(in.Models)
	if len(models) == 0 {
		return nil, fmt.Errorf("at least one model is required")
	}
	if !eff.modelsUnrestricted {
		for _, m := range models {
			if !eff.models[m] {
				return nil, fmt.Errorf("model %q is not permitted on this platform", m)
			}
		}
	}

	// Group.
	group := strings.TrimSpace(in.Group)
	if !eff.groupsUnrestricted && group != "" && !eff.groups[group] {
		return nil, fmt.Errorf("group %q is not permitted on this platform", group)
	}

	// Base URL.
	base := strings.TrimSpace(in.BaseURL)
	if base != "" {
		if err := validateBaseURL(base, eff.bases); err != nil {
			return nil, err
		}
	}

	return &Result{
		ModelsCSV: strings.Join(models, ","),
		Group:     group,
		BaseURL:   base,
	}, nil
}

func computeEffective(platform *model.Platform, grant *model.Grant) effective {
	pTypes := common.DecodeIntList(platform.AllowedTypes)
	pModels := common.DecodeStringList(platform.AllowedModels)
	pGroups := common.DecodeStringList(platform.AllowedGroups)
	bases := common.DecodeStringList(platform.BaseURLAllow)

	// Grant narrows: intersection when the grant specifies a subset.
	if grant != nil {
		if g := common.DecodeIntList(grant.AllowedTypes); len(g) > 0 {
			pTypes = intersectInts(pTypes, g)
		}
		if g := common.DecodeStringList(grant.AllowedModels); len(g) > 0 {
			pModels = intersectStrings(pModels, g)
		}
		if g := common.DecodeStringList(grant.AllowedGroups); len(g) > 0 {
			pGroups = intersectStrings(pGroups, g)
		}
	}

	return effective{
		types:              toIntSet(pTypes),
		models:             toStrSet(pModels),
		groups:             toStrSet(pGroups),
		bases:              bases,
		typesUnrestricted:  len(pTypes) == 0 && emptyGrantTypes(grant),
		modelsUnrestricted: len(pModels) == 0 && emptyGrantModels(grant),
		groupsUnrestricted: len(pGroups) == 0 && emptyGrantGroups(grant),
	}
}

// validateBaseURL requires https and, if a whitelist exists, a prefix match.
func validateBaseURL(base string, allowed []string) error {
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("base_url is not a valid absolute URL")
	}
	if u.Scheme != "https" {
		return fmt.Errorf("base_url must use https")
	}
	if len(allowed) == 0 {
		return nil
	}
	for _, a := range allowed {
		if strings.HasPrefix(base, strings.TrimSpace(a)) {
			return nil
		}
	}
	return fmt.Errorf("base_url %q is not in the platform allow-list", base)
}

// --- set helpers ---

func normalizeModels(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range in {
		m = strings.TrimSpace(m)
		if m == "" || seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, m)
	}
	return out
}

func toIntSet(xs []int) map[int]bool {
	s := make(map[int]bool, len(xs))
	for _, x := range xs {
		s[x] = true
	}
	return s
}

func toStrSet(xs []string) map[string]bool {
	s := make(map[string]bool, len(xs))
	for _, x := range xs {
		s[strings.TrimSpace(x)] = true
	}
	return s
}

func intersectInts(a, b []int) []int {
	bs := toIntSet(b)
	var out []int
	for _, x := range a {
		if bs[x] {
			out = append(out, x)
		}
	}
	return out
}

func intersectStrings(a, b []string) []string {
	bs := toStrSet(b)
	var out []string
	for _, x := range a {
		if bs[strings.TrimSpace(x)] {
			out = append(out, x)
		}
	}
	return out
}

func emptyGrantTypes(g *model.Grant) bool {
	return g == nil || len(common.DecodeIntList(g.AllowedTypes)) == 0
}
func emptyGrantModels(g *model.Grant) bool {
	return g == nil || len(common.DecodeStringList(g.AllowedModels)) == 0
}
func emptyGrantGroups(g *model.Grant) bool {
	return g == nil || len(common.DecodeStringList(g.AllowedGroups)) == 0
}
