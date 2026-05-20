package requiredresources

import (
	"context"
	"fmt"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/io41/crossplane-function-validate/input/v1alpha1"
	"github.com/io41/crossplane-function-validate/internal/celruntime"
)

var selectorAliases = []string{"claim", "context", "xr"}

func BuildRequirements(ctx context.Context, rules *v1alpha1.Rules, aliases map[string]any) (map[string]*fnv1.ResourceSelector, error) {
	if rules == nil {
		return nil, fmt.Errorf("rules is nil")
	}

	names := rules.RequiredInputNames()
	out := make(map[string]*fnv1.ResourceSelector, len(names))
	if len(names) == 0 {
		return out, nil
	}

	ev, err := celruntime.New(selectorAliases, aliases)
	if err != nil {
		return nil, fmt.Errorf("create required resource selector evaluator: %w", err)
	}

	for _, name := range names {
		spec := rules.Spec.Inputs.Required[name]

		matchName := spec.Name
		if spec.NameFrom != "" {
			matchName, err = ev.EvalString(ctx, spec.NameFrom)
			if err != nil {
				return nil, fmt.Errorf("required input %q nameFrom: %w", name, err)
			}
		}

		namespace := spec.Namespace
		if spec.NamespaceFrom != "" {
			namespace, err = ev.EvalString(ctx, spec.NamespaceFrom)
			if err != nil {
				return nil, fmt.Errorf("required input %q namespaceFrom: %w", name, err)
			}
		}

		selector := &fnv1.ResourceSelector{
			ApiVersion: spec.APIVersion,
			Kind:       spec.Kind,
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: matchName},
		}
		if namespace != "" {
			selector.Namespace = &namespace
		}
		out[name] = selector
	}

	return out, nil
}

func ApplyResolvedResources(req *fnv1.RunFunctionRequest, aliases map[string]any, names []string) (bool, error) {
	required, ok := aliases["required"].(map[string]any)
	if !ok || required == nil {
		return false, fmt.Errorf("required alias is missing or not an object")
	}

	allResolved := true
	resolved := req.GetRequiredResources()
	for _, name := range names {
		resources, ok := resolved[name]
		if !ok {
			allResolved = false
			continue
		}

		items := resources.GetItems()
		if len(items) == 0 {
			required[name] = nil
			continue
		}
		required[name] = resourceToMap(items[0])
	}

	return allResolved, nil
}

func resourceToMap(resource *fnv1.Resource) any {
	if resource.GetResource() == nil {
		return nil
	}

	return resource.GetResource().AsMap()
}
