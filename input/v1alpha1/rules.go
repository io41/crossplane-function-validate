package v1alpha1

import (
	"fmt"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	RejectWithKyvernoAuto   = "Auto"
	RejectWithKyvernoAlways = "Always"
	RejectWithKyvernoNever  = "Never"
)

type Rules struct {
	metav1.TypeMeta `json:",inline"`
	Spec            RulesSpec `json:"spec,omitempty"`
}

type RulesSpec struct {
	Inputs Inputs `json:"inputs,omitempty"`
	Rules  []Rule `json:"rules,omitempty"`
}

type Inputs struct {
	Required map[string]RequiredResource `json:"required,omitempty"`
}

type RequiredResource struct {
	APIVersion    string `json:"apiVersion,omitempty"`
	Kind          string `json:"kind,omitempty"`
	Name          string `json:"name,omitempty"`
	Namespace     string `json:"namespace,omitempty"`
	NameFrom      string `json:"nameFrom,omitempty"`
	NamespaceFrom string `json:"namespaceFrom,omitempty"`
}

type Rule struct {
	ID                string   `json:"id,omitempty"`
	Description       string   `json:"description,omitempty"`
	Uses              []string `json:"uses,omitempty"`
	RejectWithKyverno string   `json:"rejectWithKyverno,omitempty"`
	When              string   `json:"when,omitempty"`
	Assert            string   `json:"assert,omitempty"`
	Message           string   `json:"message,omitempty"`
}

func (r *Rules) DeepCopyObject() runtime.Object {
	if r == nil {
		return nil
	}

	out := *r
	out.Spec.Rules = append([]Rule(nil), r.Spec.Rules...)
	for i := range out.Spec.Rules {
		out.Spec.Rules[i].Uses = append([]string(nil), r.Spec.Rules[i].Uses...)
	}
	if r.Spec.Inputs.Required != nil {
		out.Spec.Inputs.Required = make(map[string]RequiredResource, len(r.Spec.Inputs.Required))
		for k, v := range r.Spec.Inputs.Required {
			out.Spec.Inputs.Required[k] = v
		}
	}

	return &out
}

func (r *Rules) Default() {
	if r == nil {
		return
	}

	for i := range r.Spec.Rules {
		if r.Spec.Rules[i].RejectWithKyverno == "" {
			r.Spec.Rules[i].RejectWithKyverno = RejectWithKyvernoAuto
		}
	}
}

func (r *Rules) Validate() error {
	if r == nil {
		return fmt.Errorf("input Rules is nil")
	}

	for name, rr := range r.Spec.Inputs.Required {
		if rr.APIVersion == "" {
			return fmt.Errorf("required input %q must set apiVersion", name)
		}
		if rr.Kind == "" {
			return fmt.Errorf("required input %q must set kind", name)
		}
		if rr.Name == "" && rr.NameFrom == "" {
			return fmt.Errorf("required input %q must set name or nameFrom", name)
		}
	}

	seen := map[string]struct{}{}
	for i, rule := range r.Spec.Rules {
		if rule.ID == "" {
			return fmt.Errorf("rule at index %d must set id", i)
		}
		if _, ok := seen[rule.ID]; ok {
			return fmt.Errorf("duplicate rule id %q", rule.ID)
		}
		seen[rule.ID] = struct{}{}
		if len(rule.Uses) == 0 {
			return fmt.Errorf("rule %q must set uses", rule.ID)
		}
		if rule.Assert == "" {
			return fmt.Errorf("rule %q must set assert", rule.ID)
		}
		if rule.Message == "" {
			return fmt.Errorf("rule %q must set message", rule.ID)
		}
		switch rule.RejectWithKyverno {
		case RejectWithKyvernoAuto, RejectWithKyvernoAlways, RejectWithKyvernoNever:
		default:
			return fmt.Errorf("rule %q has invalid rejectWithKyverno %q", rule.ID, rule.RejectWithKyverno)
		}
	}

	return nil
}

func (r *Rules) RequiredInputNames() []string {
	if r == nil {
		return nil
	}

	names := make([]string, 0, len(r.Spec.Inputs.Required))
	for name := range r.Spec.Inputs.Required {
		names = append(names, name)
	}
	sort.Strings(names)

	return names
}
