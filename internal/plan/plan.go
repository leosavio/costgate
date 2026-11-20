// Package plan reads the JSON output of `terraform show -json <planfile>`
// (also produced by `tofu show -json`).
package plan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Plan is the subset of the plan representation costgate uses.
type Plan struct {
	FormatVersion    string           `json:"format_version"`
	TerraformVersion string           `json:"terraform_version"`
	ResourceChanges  []ResourceChange `json:"resource_changes"`
	Configuration    Configuration    `json:"configuration"`
	// Variables are the input variables of the root module, with the
	// values of this plan.
	Variables map[string]struct {
		Value any `json:"value"`
	} `json:"variables"`

	// configKeys maps a resource address without instance keys
	// ("module.vpc.aws_eip.nat") to its provider_config_key.
	configKeys map[string]string
}

// Configuration is the subset of the "configuration" section costgate uses:
// provider configurations and the provider of each configured resource.
type Configuration struct {
	ProviderConfig map[string]ProviderConfig `json:"provider_config"`
	RootModule     ConfigModule              `json:"root_module"`
}

// ProviderConfig is one entry of configuration.provider_config.
type ProviderConfig struct {
	Name          string         `json:"name"`
	FullName      string         `json:"full_name"`
	Alias         string         `json:"alias"`
	ModuleAddress string         `json:"module_address"`
	Expressions   map[string]any `json:"expressions"`
}

// ConfigModule is a module of the configuration tree.
type ConfigModule struct {
	Resources   []ConfigResource      `json:"resources"`
	ModuleCalls map[string]ModuleCall `json:"module_calls"`
}

// ConfigResource is a resource block of the configuration. Address is
// relative to its module.
type ConfigResource struct {
	Address           string `json:"address"`
	Mode              string `json:"mode"`
	ProviderConfigKey string `json:"provider_config_key"`
}

// ModuleCall is a module block of the configuration.
type ModuleCall struct {
	Module ConfigModule `json:"module"`
}

// ResourceChange is one entry of resource_changes.
type ResourceChange struct {
	Address      string `json:"address"`
	Mode         string `json:"mode"`
	Type         string `json:"type"`
	Name         string `json:"name"`
	ProviderName string `json:"provider_name"`
	Change       Change `json:"change"`
}

// Change carries the before/after values. Numbers are json.Number.
type Change struct {
	Actions      []string `json:"actions"`
	Before       any      `json:"before"`
	After        any      `json:"after"`
	AfterUnknown any      `json:"after_unknown"`
}

// Action is the normalized form of Change.Actions.
type Action string

// Normalized actions.
const (
	NoOp    Action = "no-op"
	Create  Action = "create"
	Update  Action = "update"
	Delete  Action = "delete"
	Replace Action = "replace"
	Read    Action = "read"
	Forget  Action = "forget"
)

// Normalize maps the actions list of the plan format to one Action.
func Normalize(actions []string) (Action, error) {
	switch strings.Join(actions, ",") {
	case "no-op":
		return NoOp, nil
	case "create":
		return Create, nil
	case "update":
		return Update, nil
	case "delete":
		return Delete, nil
	case "delete,create", "create,delete":
		return Replace, nil
	case "read":
		return Read, nil
	case "forget":
		return Forget, nil
	}
	return "", fmt.Errorf("unsupported actions %v", actions)
}

// Parse decodes a plan and rejects inputs that are not a plan.
func Parse(r io.Reader) (*Plan, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("plan: not JSON: %w", err)
	}
	if _, ok := probe["planned_values"]; !ok {
		if _, ok := probe["resource_changes"]; !ok {
			return nil, fmt.Errorf("plan: no planned_values nor resource_changes; pass the output of 'terraform show -json <planfile>', not of a state")
		}
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var p Plan
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("plan: %w", err)
	}
	major, _, _ := strings.Cut(p.FormatVersion, ".")
	if major != "1" {
		return nil, fmt.Errorf("plan: format_version %q not supported (this build reads 1.x)", p.FormatVersion)
	}
	sort.SliceStable(p.ResourceChanges, func(i, j int) bool {
		return p.ResourceChanges[i].Address < p.ResourceChanges[j].Address
	})
	p.configKeys = map[string]string{}
	indexConfig(p.configKeys, "", p.Configuration.RootModule)
	return &p, nil
}

func indexConfig(keys map[string]string, prefix string, m ConfigModule) {
	for _, r := range m.Resources {
		if r.ProviderConfigKey != "" {
			keys[prefix+r.Address] = r.ProviderConfigKey
		}
	}
	for name, call := range m.ModuleCalls {
		indexConfig(keys, prefix+"module."+name+".", call.Module)
	}
}

// ProviderConfigKey returns the provider_config_key of the configuration
// block behind a resource change address, or "" when the plan has no
// configuration section for it.
func (p *Plan) ProviderConfigKey(address string) string {
	return p.configKeys[ConfigAddress(address)]
}

// ProviderRegion returns the region of a provider configuration when the
// plan states it: a constant (region = "us-west-2"), or a root module
// variable alone (region = var.region), whose value is in the plan. A region
// from a local, a module variable, an expression or an environment variable
// is not in the plan.
func (p *Plan) ProviderRegion(key string) (string, bool) {
	pc, ok := p.Configuration.ProviderConfig[key]
	if !ok {
		return "", false
	}
	expr, _ := pc.Expressions["region"].(map[string]any)
	if r, _ := expr["constant_value"].(string); r != "" {
		return r, true
	}
	refs, _ := expr["references"].([]any)
	if pc.ModuleAddress != "" || len(refs) != 1 {
		return "", false
	}
	ref, _ := refs[0].(string)
	name, ok := strings.CutPrefix(ref, "var.")
	if !ok || name == "" || strings.ContainsAny(name, ".[") {
		return "", false
	}
	r, _ := p.Variables[name].Value.(string)
	return r, r != ""
}

// ConfigAddress removes the instance keys of an address:
// module.eks["a"].aws_kms_key.this[0] becomes module.eks.aws_kms_key.this.
func ConfigAddress(address string) string {
	var b strings.Builder
	depth, inString, escaped := 0, false, false
	for _, c := range address {
		switch {
		case inString:
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
		case depth > 0:
			switch c {
			case '"':
				inString = true
			case '[':
				depth++
			case ']':
				depth--
			}
		case c == '[':
			depth = 1
		default:
			b.WriteRune(c)
		}
	}
	return b.String()
}
