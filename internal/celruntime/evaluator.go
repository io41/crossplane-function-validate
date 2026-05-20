package celruntime

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

const (
	defaultCostLimit uint64 = 100000
	defaultTimeout          = 2 * time.Second
)

type Evaluator struct {
	env    *cel.Env
	values map[string]any
}

func New(aliases []string, values map[string]any) (*Evaluator, error) {
	roots, err := aliasRoots(aliases)
	if err != nil {
		return nil, err
	}

	options := make([]cel.EnvOption, 0, len(roots))
	evalValues := make(map[string]any, len(roots))
	for _, root := range roots {
		options = append(options, cel.Variable(root, cel.DynType))
		evalValues[root] = values[root]
	}

	env, err := cel.NewEnv(options...)
	if err != nil {
		return nil, fmt.Errorf("create CEL environment: %w", err)
	}

	return &Evaluator{env: env, values: evalValues}, nil
}

func (e *Evaluator) EvalBool(ctx context.Context, expr string) (bool, error) {
	out, err := e.eval(ctx, expr)
	if err != nil {
		return false, err
	}

	b, ok := out.Value().(bool)
	if !ok {
		return false, fmt.Errorf("expression %q must evaluate to bool, got %T", expr, out.Value())
	}

	return b, nil
}

func (e *Evaluator) EvalString(ctx context.Context, expr string) (string, error) {
	out, err := e.eval(ctx, expr)
	if err != nil {
		return "", err
	}

	s, ok := out.Value().(string)
	if !ok {
		return "", fmt.Errorf("expression %q must evaluate to string, got %T", expr, out.Value())
	}

	return s, nil
}

func (e *Evaluator) eval(ctx context.Context, expr string) (ref.Val, error) {
	if e == nil {
		return nil, fmt.Errorf("CEL evaluator is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	ast, issues := e.env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("compile CEL expression %q: %w", expr, issues.Err())
	}

	program, err := e.env.Program(ast, cel.CostLimit(defaultCostLimit))
	if err != nil {
		return nil, fmt.Errorf("create CEL program for %q: %w", expr, err)
	}

	runCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	out, _, err := program.ContextEval(runCtx, e.values)
	if err != nil {
		return nil, fmt.Errorf("evaluate CEL expression %q: %w", expr, err)
	}
	if out == nil {
		return nil, fmt.Errorf("evaluate CEL expression %q: no result", expr)
	}
	if types.IsError(out) {
		return nil, fmt.Errorf("evaluate CEL expression %q: %v", expr, out)
	}

	return out, nil
}

func aliasRoots(aliases []string) ([]string, error) {
	seen := map[string]struct{}{}
	for _, alias := range aliases {
		root := aliasRoot(alias)
		if root == "" {
			return nil, fmt.Errorf("alias must not be empty")
		}
		seen[root] = struct{}{}
	}

	roots := make([]string, 0, len(seen))
	for root := range seen {
		roots = append(roots, root)
	}
	sort.Strings(roots)

	return roots, nil
}

func aliasRoot(alias string) string {
	root, _, _ := strings.Cut(alias, ".")
	return root
}
