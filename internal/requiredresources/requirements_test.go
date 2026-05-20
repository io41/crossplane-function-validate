package requiredresources

import (
	"context"
	"testing"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/io41/crossplane-function-validate/input/v1alpha1"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestBuildRequirementsResolvesNameFromAndNamespaceFrom(t *testing.T) {
	rules := &v1alpha1.Rules{Spec: v1alpha1.RulesSpec{Inputs: v1alpha1.Inputs{Required: map[string]v1alpha1.RequiredResource{
		"namespace": {
			APIVersion:    "example.org/v1",
			Kind:          "XNamespace",
			NameFrom:      "xr.spec.serviceBusRef.name",
			NamespaceFrom: "xr.spec.serviceBusRef.namespace",
		},
	}}}}
	aliases := map[string]any{
		"xr": map[string]any{"spec": map[string]any{"serviceBusRef": map[string]any{
			"name":      "bus",
			"namespace": "platform",
		}}},
	}

	reqs, err := BuildRequirements(context.Background(), rules, aliases)
	if err != nil {
		t.Fatal(err)
	}

	got := reqs["namespace"]
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
}

func TestBuildRequirementsAllowsLiteralNameAndNamespace(t *testing.T) {
	rules := &v1alpha1.Rules{Spec: v1alpha1.RulesSpec{Inputs: v1alpha1.Inputs{Required: map[string]v1alpha1.RequiredResource{
		"namespace": {
			APIVersion: "example.org/v1",
			Kind:       "XNamespace",
			Name:       "bus",
			Namespace:  "platform",
		},
	}}}}

	reqs, err := BuildRequirements(context.Background(), rules, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}

	got := reqs["namespace"]
	if got.GetMatchName() != "bus" {
		t.Fatalf("matchName = %q, want bus", got.GetMatchName())
	}
	if got.GetNamespace() != "platform" {
		t.Fatalf("namespace = %q, want platform", got.GetNamespace())
	}
}

func TestBuildRequirementsRejectsForbiddenSelectorAliases(t *testing.T) {
	for _, expr := range []string{
		"claim.metadata.name",
		"required.namespace.metadata.name",
		"observed.composite.metadata.name",
		"desired.composite.metadata.name",
	} {
		t.Run(expr, func(t *testing.T) {
			rules := &v1alpha1.Rules{Spec: v1alpha1.RulesSpec{Inputs: v1alpha1.Inputs{Required: map[string]v1alpha1.RequiredResource{
				"namespace": {
					APIVersion:    "example.org/v1",
					Kind:          "XNamespace",
					NameFrom:      expr,
					NamespaceFrom: `"platform"`,
				},
			}}}}
			aliases := map[string]any{
				"claim":    map[string]any{"metadata": map[string]any{"name": "bus"}},
				"required": map[string]any{"namespace": map[string]any{"metadata": map[string]any{"name": "bus"}}},
				"observed": map[string]any{"composite": map[string]any{"metadata": map[string]any{"name": "bus"}}},
				"desired":  map[string]any{"composite": map[string]any{"metadata": map[string]any{"name": "bus"}}},
			}

			if _, err := BuildRequirements(context.Background(), rules, aliases); err == nil {
				t.Fatal("BuildRequirements() error = nil, want forbidden alias error")
			}
		})
	}
}

func TestBuildRequirementsRejectsMissingNamespace(t *testing.T) {
	rules := &v1alpha1.Rules{Spec: v1alpha1.RulesSpec{Inputs: v1alpha1.Inputs{Required: map[string]v1alpha1.RequiredResource{
		"namespace": {
			APIVersion: "example.org/v1",
			Kind:       "XNamespace",
			Name:       "bus",
		},
	}}}}

	if _, err := BuildRequirements(context.Background(), rules, map[string]any{}); err == nil {
		t.Fatal("BuildRequirements() error = nil, want missing namespace error")
	}
}

func TestApplyResolvedResourcesSetsNullWhenResolvedButNotFound(t *testing.T) {
	aliases := map[string]any{"required": map[string]any{}}
	req := &fnv1.RunFunctionRequest{RequiredResources: map[string]*fnv1.Resources{
		"namespace": {},
	}}

	resolved, err := ApplyResolvedResources(req, aliases, []string{"namespace"})
	if err != nil {
		t.Fatal(err)
	}
	if !resolved {
		t.Fatal("resolved = false, want true")
	}

	required := aliases["required"].(map[string]any)
	if _, ok := required["namespace"]; !ok {
		t.Fatal("required.namespace missing")
	}
	if required["namespace"] != nil {
		t.Fatalf("required.namespace = %#v, want nil", required["namespace"])
	}
}

func TestApplyResolvedResourcesReportsPendingWhenMissingFromRequest(t *testing.T) {
	aliases := map[string]any{"required": map[string]any{}}

	resolved, err := ApplyResolvedResources(&fnv1.RunFunctionRequest{}, aliases, []string{"namespace"})
	if err != nil {
		t.Fatal(err)
	}
	if resolved {
		t.Fatal("resolved = true, want false")
	}

	required := aliases["required"].(map[string]any)
	if _, ok := required["namespace"]; ok {
		t.Fatal("required.namespace set, want missing alias while request is pending")
	}
}

func TestApplyResolvedResourcesUsesFirstResolvedItem(t *testing.T) {
	aliases := map[string]any{"required": map[string]any{}}
	req := &fnv1.RunFunctionRequest{RequiredResources: map[string]*fnv1.Resources{
		"namespace": {Items: []*fnv1.Resource{
			{Resource: mustStruct(t, map[string]any{
				"apiVersion": "example.org/v1",
				"kind":       "XNamespace",
				"metadata":   map[string]any{"name": "bus"},
			})},
			{Resource: mustStruct(t, map[string]any{
				"metadata": map[string]any{"name": "ignored"},
			})},
		}},
	}}

	resolved, err := ApplyResolvedResources(req, aliases, []string{"namespace"})
	if err != nil {
		t.Fatal(err)
	}
	if !resolved {
		t.Fatal("resolved = false, want true")
	}

	required := aliases["required"].(map[string]any)
	namespace := required["namespace"].(map[string]any)
	metadata := namespace["metadata"].(map[string]any)
	if got := metadata["name"]; got != "bus" {
		t.Fatalf("required.namespace.metadata.name = %v, want bus", got)
	}
}

func TestApplyResolvedResourcesPreservesNilResourcePayload(t *testing.T) {
	aliases := map[string]any{"required": map[string]any{}}
	req := &fnv1.RunFunctionRequest{RequiredResources: map[string]*fnv1.Resources{
		"namespace": {Items: []*fnv1.Resource{{}}},
	}}

	resolved, err := ApplyResolvedResources(req, aliases, []string{"namespace"})
	if err != nil {
		t.Fatal(err)
	}
	if !resolved {
		t.Fatal("resolved = false, want true")
	}

	required := aliases["required"].(map[string]any)
	if got := required["namespace"]; got != nil {
		t.Fatalf("required.namespace = %#v, want nil", got)
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
