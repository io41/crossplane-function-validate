package validation

import (
	"context"
	"testing"

	"github.com/io41/crossplane-function-validate/input/v1alpha1"
)

func TestEvaluateReturnsFailuresInRuleOrder(t *testing.T) {
	rules := &v1alpha1.Rules{Spec: v1alpha1.RulesSpec{Rules: []v1alpha1.Rule{
		{ID: "first", Uses: []string{"xr"}, Assert: "false", Message: "first failed"},
		{ID: "second", Uses: []string{"xr"}, Assert: "false", Message: "second failed"},
	}}}
	rules.Default()
	out, err := Evaluate(context.Background(), rules, map[string]any{"xr": map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Failures) != 2 {
		t.Fatalf("failures = %d, want 2", len(out.Failures))
	}
	if out.Failures[0].Message != "first failed" || out.Failures[1].Message != "second failed" {
		t.Fatalf("failure order = %#v", out.Failures)
	}
}

func TestEvaluateSkipsWhenFalse(t *testing.T) {
	rules := &v1alpha1.Rules{Spec: v1alpha1.RulesSpec{Rules: []v1alpha1.Rule{{
		ID: "skip", Uses: []string{"xr"}, When: "false", Assert: "false", Message: "must not appear",
	}}}}
	rules.Default()
	out, err := Evaluate(context.Background(), rules, map[string]any{"xr": map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Failures) != 0 {
		t.Fatalf("failures = %#v, want none", out.Failures)
	}
	if len(out.Skipped) != 1 || out.Skipped[0] != "skip" {
		t.Fatalf("skipped = %#v, want [skip]", out.Skipped)
	}
}

func TestEvaluateRejectsUndeclaredAlias(t *testing.T) {
	rules := &v1alpha1.Rules{Spec: v1alpha1.RulesSpec{Rules: []v1alpha1.Rule{{
		ID: "bad", Uses: []string{"xr"}, Assert: "required.namespace != null", Message: "bad",
	}}}}
	rules.Default()
	_, err := Evaluate(context.Background(), rules, map[string]any{"xr": map[string]any{}, "required": map[string]any{"namespace": nil}})
	if err == nil {
		t.Fatal("Evaluate() error = nil, want undeclared alias error")
	}
}
