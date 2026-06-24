package validate

import (
	"testing"

	"github.com/modex/agt-vault/common"
	"github.com/modex/agt-vault/constant"
	"github.com/modex/agt-vault/model"
)

func platformWith(types []int, models, groups, bases []string) *model.Platform {
	return &model.Platform{
		AllowedTypes:  common.EncodeJSON(types),
		AllowedModels: common.EncodeJSON(models),
		AllowedGroups: common.EncodeJSON(groups),
		BaseURLAllow:  common.EncodeJSON(bases),
	}
}

func TestChannelUpload_TypeWhitelist(t *testing.T) {
	p := platformWith([]int{constant.ChannelTypeOpenAI}, []string{"gpt-4o"}, nil, nil)
	g := &model.Grant{}

	// Permitted type.
	if _, err := ChannelUpload(ChannelInput{Type: constant.ChannelTypeOpenAI, Models: []string{"gpt-4o"}}, p, g); err != nil {
		t.Errorf("permitted type rejected: %v", err)
	}
	// Forbidden type (Anthropic not in platform whitelist).
	if _, err := ChannelUpload(ChannelInput{Type: constant.ChannelTypeAnthropic, Models: []string{"gpt-4o"}}, p, g); err == nil {
		t.Error("forbidden type accepted")
	}
	// Unsupported type entirely.
	if _, err := ChannelUpload(ChannelInput{Type: 999, Models: []string{"x"}}, p, g); err == nil {
		t.Error("unsupported type accepted")
	}
}

func TestChannelUpload_ModelWhitelist(t *testing.T) {
	p := platformWith([]int{constant.ChannelTypeOpenAI}, []string{"gpt-4o", "gpt-4o-mini"}, nil, nil)
	g := &model.Grant{}

	if _, err := ChannelUpload(ChannelInput{Type: constant.ChannelTypeOpenAI, Models: []string{"gpt-4o"}}, p, g); err != nil {
		t.Errorf("permitted model rejected: %v", err)
	}
	if _, err := ChannelUpload(ChannelInput{Type: constant.ChannelTypeOpenAI, Models: []string{"gpt-4o", "o1-pro"}}, p, g); err == nil {
		t.Error("forbidden model accepted")
	}
}

func TestChannelUpload_GrantNarrowsPlatform(t *testing.T) {
	// Platform allows two types; grant narrows to just OpenAI.
	p := platformWith([]int{constant.ChannelTypeOpenAI, constant.ChannelTypeAnthropic},
		[]string{"gpt-4o", "claude-3-5-sonnet"}, nil, nil)
	g := &model.Grant{AllowedTypes: common.EncodeJSON([]int{constant.ChannelTypeOpenAI})}

	if _, err := ChannelUpload(ChannelInput{Type: constant.ChannelTypeOpenAI, Models: []string{"gpt-4o"}}, p, g); err != nil {
		t.Errorf("grant-permitted type rejected: %v", err)
	}
	// Anthropic is platform-allowed but grant-narrowed-out.
	if _, err := ChannelUpload(ChannelInput{Type: constant.ChannelTypeAnthropic, Models: []string{"claude-3-5-sonnet"}}, p, g); err == nil {
		t.Error("grant narrowing not enforced: Anthropic accepted")
	}
}

func TestChannelUpload_BaseURL(t *testing.T) {
	p := platformWith([]int{constant.ChannelTypeOpenAI}, []string{"gpt-4o"}, nil,
		[]string{"https://api.openai.com"})
	g := &model.Grant{}

	base := func(u string) ChannelInput {
		return ChannelInput{Type: constant.ChannelTypeOpenAI, Models: []string{"gpt-4o"}, BaseURL: u}
	}
	if _, err := ChannelUpload(base("https://api.openai.com/v1"), p, g); err != nil {
		t.Errorf("allowed base_url rejected: %v", err)
	}
	if _, err := ChannelUpload(base("https://evil.example.com"), p, g); err == nil {
		t.Error("base_url outside allow-list accepted")
	}
	if _, err := ChannelUpload(base("http://api.openai.com"), p, g); err == nil {
		t.Error("non-https base_url accepted")
	}
}

func TestChannelUpload_NormalizesModels(t *testing.T) {
	p := platformWith(nil, nil, nil, nil) // unrestricted
	g := &model.Grant{}
	res, err := ChannelUpload(ChannelInput{
		Type: constant.ChannelTypeOpenAI, Models: []string{" gpt-4o ", "gpt-4o", "", "gpt-4o-mini"},
	}, p, g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ModelsCSV != "gpt-4o,gpt-4o-mini" {
		t.Errorf("normalized models = %q, want %q", res.ModelsCSV, "gpt-4o,gpt-4o-mini")
	}
}
