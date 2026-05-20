package validation

import (
	"context"
	"fmt"

	"github.com/io41/crossplane-function-validate/input/v1alpha1"
	"github.com/io41/crossplane-function-validate/internal/celruntime"
)

type Outcome struct {
	Failures []Failure
	Skipped  []string
}

type Failure struct {
	RuleID  string
	Message string
}

func Evaluate(ctx context.Context, rules *v1alpha1.Rules, aliases map[string]any) (*Outcome, error) {
	out := &Outcome{}
	for _, rule := range rules.Spec.Rules {
		ev, err := celruntime.New(rule.Uses, aliases)
		if err != nil {
			return nil, fmt.Errorf("rule %q: %w", rule.ID, err)
		}
		if rule.When != "" {
			ok, err := ev.EvalBool(ctx, rule.When)
			if err != nil {
				return nil, fmt.Errorf("rule %q when: %w", rule.ID, err)
			}
			if !ok {
				out.Skipped = append(out.Skipped, rule.ID)
				continue
			}
		}
		ok, err := ev.EvalBool(ctx, rule.Assert)
		if err != nil {
			return nil, fmt.Errorf("rule %q assert: %w", rule.ID, err)
		}
		if !ok {
			out.Failures = append(out.Failures, Failure{RuleID: rule.ID, Message: rule.Message})
		}
	}
	return out, nil
}
