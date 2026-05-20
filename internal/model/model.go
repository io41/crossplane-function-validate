package model

import (
	"fmt"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"google.golang.org/protobuf/types/known/structpb"
)

type Model struct {
	Aliases map[string]any
}

func Build(req *fnv1.RunFunctionRequest) (*Model, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}

	aliases := map[string]any{
		"claim":    nil,
		"context":  structToMap(req.GetContext()),
		"observed": stateToMap(req.GetObserved()),
		"desired":  stateToMap(req.GetDesired()),
		"required": map[string]any{},
	}
	if req.GetObserved() != nil && req.GetObserved().GetComposite() != nil {
		aliases["xr"] = structToMap(req.GetObserved().GetComposite().GetResource())
	} else {
		aliases["xr"] = nil
	}

	return &Model{Aliases: aliases}, nil
}

func stateToMap(s *fnv1.State) map[string]any {
	out := map[string]any{
		"composite": nil,
		"resources": map[string]any{},
	}
	if s == nil {
		return out
	}
	if s.GetComposite() != nil {
		out["composite"] = structToMap(s.GetComposite().GetResource())
	}

	resources := map[string]any{}
	for name, resource := range s.GetResources() {
		resources[name] = structToMap(resource.GetResource())
	}
	out["resources"] = resources

	return out
}

func structToMap(s *structpb.Struct) map[string]any {
	if s == nil {
		return map[string]any{}
	}

	return s.AsMap()
}
