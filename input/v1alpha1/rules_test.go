package v1alpha1

import (
	"strings"
	"testing"
)

func TestRulesDefaultRejectWithKyverno(t *testing.T) {
	r := &Rules{Spec: RulesSpec{Rules: []Rule{{ID: "r1", Uses: []string{"xr"}, Assert: "true", Message: "ok"}}}}
	r.Default()
	if got := r.Spec.Rules[0].RejectWithKyverno; got != RejectWithKyvernoAuto {
		t.Fatalf("default rejectWithKyverno = %q, want %q", got, RejectWithKyvernoAuto)
	}
}

func TestRulesValidateRejectsDuplicateIDs(t *testing.T) {
	r := &Rules{Spec: RulesSpec{Rules: []Rule{
		{ID: "same", Uses: []string{"xr"}, Assert: "true", Message: "one"},
		{ID: "same", Uses: []string{"xr"}, Assert: "true", Message: "two"},
	}}}
	r.Default()
	err := r.Validate()
	if err == nil || !strings.Contains(err.Error(), `duplicate rule id "same"`) {
		t.Fatalf("Validate() error = %v, want duplicate rule id", err)
	}
}

func TestRulesValidateRejectsInvalidKyvernoMode(t *testing.T) {
	r := &Rules{Spec: RulesSpec{Rules: []Rule{{
		ID: "r1", Uses: []string{"xr"}, RejectWithKyverno: "Soon", Assert: "true", Message: "ok",
	}}}}
	err := r.Validate()
	if err == nil || !strings.Contains(err.Error(), `rejectWithKyverno`) {
		t.Fatalf("Validate() error = %v, want rejectWithKyverno error", err)
	}
}

func TestRulesValidateRejectsRequiredInputWithoutName(t *testing.T) {
	r := &Rules{Spec: RulesSpec{Inputs: Inputs{Required: map[string]RequiredResource{
		"namespace": {APIVersion: "example.org/v1", Kind: "XNamespace", Namespace: "platform"},
	}}, Rules: []Rule{{ID: "r1", Uses: []string{"required.namespace"}, Assert: "true", Message: "ok"}}}}
	err := r.Validate()
	if err == nil || !strings.Contains(err.Error(), `required input "namespace" must set name or nameFrom`) {
		t.Fatalf("Validate() error = %v, want required input name error", err)
	}
}

func TestRulesValidateRejectsRequiredInputWithoutNamespace(t *testing.T) {
	r := &Rules{Spec: RulesSpec{Inputs: Inputs{Required: map[string]RequiredResource{
		"namespace": {APIVersion: "example.org/v1", Kind: "XNamespace", Name: "bus"},
	}}, Rules: []Rule{{ID: "r1", Uses: []string{"required.namespace"}, Assert: "true", Message: "ok"}}}}
	err := r.Validate()
	if err == nil || !strings.Contains(err.Error(), `required input "namespace" must set namespace or namespaceFrom`) {
		t.Fatalf("Validate() error = %v, want required input namespace error", err)
	}
}

func TestRulesValidateRejectsClaimAlias(t *testing.T) {
	r := &Rules{Spec: RulesSpec{Rules: []Rule{{
		ID: "claim", Uses: []string{"claim"}, Assert: "claim != null", Message: "ok",
	}}}}
	r.Default()
	err := r.Validate()
	if err == nil || !strings.Contains(err.Error(), `claim is not available in v1`) {
		t.Fatalf("Validate() error = %v, want unsupported claim alias error", err)
	}
}
