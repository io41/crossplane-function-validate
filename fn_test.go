package main

import (
	"context"
	"strings"
	"testing"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestRunFunctionReturnsRequiredResourceSelectors(t *testing.T) {
	input := mustStruct(t, map[string]any{
		"apiVersion": "validate.fn.crossplane.io/v1alpha1",
		"kind":       "Rules",
		"spec": map[string]any{
			"inputs": map[string]any{"required": map[string]any{"namespace": map[string]any{
				"apiVersion":    "example.org/v1",
				"kind":          "XNamespace",
				"nameFrom":      "xr.spec.ref.name",
				"namespaceFrom": "xr.spec.ref.namespace",
			}}},
			"rules": []any{map[string]any{
				"id":      "exists",
				"uses":    []any{"required.namespace"},
				"assert":  "required.namespace != null",
				"message": "namespace missing",
			}},
		},
	})
	req := &fnv1.RunFunctionRequest{
		Input: input,
		Observed: &fnv1.State{Composite: &fnv1.Resource{Resource: mustStruct(t, map[string]any{
			"spec": map[string]any{"ref": map[string]any{
				"name":      "bus",
				"namespace": "platform",
			}},
		})}},
	}

	rsp, err := (&Function{}).RunFunction(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	got := rsp.GetRequirements().GetResources()["namespace"]
	if got == nil {
		t.Fatal("requirements.resources[namespace] missing")
	}
	if got.GetApiVersion() != "example.org/v1" {
		t.Fatalf("apiVersion = %q, want example.org/v1", got.GetApiVersion())
	}
	if got.GetKind() != "XNamespace" {
		t.Fatalf("kind = %q, want XNamespace", got.GetKind())
	}
	if got.GetMatchName() != "bus" {
		t.Fatalf("matchName = %q, want bus", got.GetMatchName())
	}
	if got.GetNamespace() != "platform" {
		t.Fatalf("namespace = %q, want platform", got.GetNamespace())
	}
	if len(rsp.GetResults()) != 0 {
		t.Fatalf("results = %#v, want none while required resource is pending", rsp.GetResults())
	}
}

func TestRunFunctionAggregatesValidationFailures(t *testing.T) {
	input := mustStruct(t, map[string]any{
		"apiVersion": "validate.fn.crossplane.io/v1alpha1",
		"kind":       "Rules",
		"spec": map[string]any{"rules": []any{
			map[string]any{"id": "one", "uses": []any{"xr"}, "assert": "false", "message": "first failed"},
			map[string]any{"id": "two", "uses": []any{"xr"}, "assert": "false", "message": "second failed"},
		}},
	})
	req := &fnv1.RunFunctionRequest{
		Input: input,
		Observed: &fnv1.State{Composite: &fnv1.Resource{Resource: mustStruct(t, map[string]any{
			"spec": map[string]any{},
		})}},
	}

	rsp, err := (&Function{}).RunFunction(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(rsp.GetResults()) != 1 {
		t.Fatalf("results = %d, want 1", len(rsp.GetResults()))
	}
	result := rsp.GetResults()[0]
	if result.GetSeverity() != fnv1.Severity_SEVERITY_FATAL {
		t.Fatalf("severity = %v, want fatal", result.GetSeverity())
	}
	msg := result.GetMessage()
	for _, want := range []string{"validation failed:", "- first failed", "- second failed"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("fatal message = %q, want %q", msg, want)
		}
	}
	if strings.Index(msg, "first failed") > strings.Index(msg, "second failed") {
		t.Fatalf("fatal message = %q, want failures in input order", msg)
	}
}

func mustStruct(t *testing.T, m map[string]any) *structpb.Struct {
	t.Helper()
	s, err := structpb.NewStruct(m)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
