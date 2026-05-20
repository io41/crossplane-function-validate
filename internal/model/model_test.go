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

func mustStruct(t *testing.T, m map[string]any) *structpb.Struct {
	t.Helper()
	s, err := structpb.NewStruct(m)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
