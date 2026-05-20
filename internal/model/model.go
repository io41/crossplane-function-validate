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
		"context":  contextToMap(req.GetContext()),
		"observed": stateToMap(req.GetObserved()),
		"desired":  stateToMap(req.GetDesired()),
		"required": map[string]any{},
	}
	if req.GetObserved() != nil && req.GetObserved().GetComposite() != nil {
		aliases["xr"] = resourceToMap(req.GetObserved().GetComposite().GetResource())
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
		out["composite"] = resourceToMap(s.GetComposite().GetResource())
	}

	resources := map[string]any{}
	for name, resource := range s.GetResources() {
		if resource == nil {
			resources[name] = nil
			continue
		}
		resources[name] = resourceToMap(resource.GetResource())
	}
	out["resources"] = resources

	return out
}

func contextToMap(s *structpb.Struct) map[string]any {
	if s == nil {
		return map[string]any{}
	}

	return s.AsMap()
}

func resourceToMap(s *structpb.Struct) any {
	if s == nil {
		return nil
	}

	return s.AsMap()
}
