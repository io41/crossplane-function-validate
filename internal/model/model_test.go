package model

import (
	"testing"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestBuildBindsAbsentClaimToNil(t *testing.T) {
	req := &fnv1.RunFunctionRequest{
		Observed: &fnv1.State{Composite: &fnv1.Resource{Resource: mustStruct(t, map[string]any{
			"apiVersion": "example.org/v1",
			"kind":       "XR",
			"spec":       map[string]any{"environment": "dev"},
		})}},
	}
	m, err := Build(req)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Aliases["claim"]; !ok {
		t.Fatal("claim alias missing")
	}
	if m.Aliases["claim"] != nil {
		t.Fatalf("claim alias = %#v, want nil", m.Aliases["claim"])
	}
}

func TestBuildExposesXRSpec(t *testing.T) {
	req := &fnv1.RunFunctionRequest{
		Observed: &fnv1.State{Composite: &fnv1.Resource{Resource: mustStruct(t, map[string]any{
			"apiVersion": "example.org/v1",
			"kind":       "XR",
			"spec":       map[string]any{"environment": "dev"},
		})}},
	}
	m, err := Build(req)
	if err != nil {
		t.Fatal(err)
	}
	xr := m.Aliases["xr"].(map[string]any)
	spec := xr["spec"].(map[string]any)
	if got := spec["environment"]; got != "dev" {
		t.Fatalf("environment = %v, want dev", got)
	}
}

func TestBuildPreservesNilResourcePayloads(t *testing.T) {
	req := &fnv1.RunFunctionRequest{
		Observed: &fnv1.State{
			Composite: &fnv1.Resource{},
			Resources: map[string]*fnv1.Resource{
				"empty": {},
			},
		},
		Desired: &fnv1.State{Composite: &fnv1.Resource{}},
	}
	m, err := Build(req)
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Aliases["xr"]; got != nil {
		t.Fatalf("xr = %#v, want nil", got)
	}
	if got := m.Aliases["context"]; len(got.(map[string]any)) != 0 {
		t.Fatalf("context = %#v, want empty map", got)
	}

	observed := m.Aliases["observed"].(map[string]any)
	if got := observed["composite"]; got != nil {
		t.Fatalf("observed.composite = %#v, want nil", got)
	}
	observedResources := observed["resources"].(map[string]any)
	if got := observedResources["empty"]; got != nil {
		t.Fatalf("observed.resources.empty = %#v, want nil", got)
	}

	desired := m.Aliases["desired"].(map[string]any)
	if got := desired["composite"]; got != nil {
		t.Fatalf("desired.composite = %#v, want nil", got)
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
