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
	return &p, nil
}
