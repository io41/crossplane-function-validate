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
	defaultCostLimit               uint64 = 100000
	defaultInterruptCheckFrequency uint   = 100
	defaultTimeout                        = 2 * time.Second
)

type Evaluator struct {
	env    *cel.Env
	values map[string]any
	limits evaluatorLimits
}

type evaluatorLimits struct {
	costLimit               uint64
	interruptCheckFrequency uint
	timeout                 time.Duration
}

func New(aliases []string, values map[string]any) (*Evaluator, error) {
	return newWithLimits(aliases, values, evaluatorLimits{
		costLimit:               defaultCostLimit,
		interruptCheckFrequency: defaultInterruptCheckFrequency,
		timeout:                 defaultTimeout,
	})
}

func newWithLimits(aliases []string, values map[string]any, limits evaluatorLimits) (*Evaluator, error) {
	roots, aliasTree, err := buildAliasTree(aliases)
	if err != nil {
		return nil, err
	}
	if limits.interruptCheckFrequency == 0 {
		return nil, fmt.Errorf("interrupt check frequency must be greater than 0")
	}
	if limits.timeout <= 0 {
		return nil, fmt.Errorf("timeout must be greater than 0")
	}

	options := make([]cel.EnvOption, 0, len(roots))
	for _, root := range roots {
		options = append(options, cel.Variable(root, cel.DynType))
	}
	evalValues := pruneActivationValues(values, roots, aliasTree)

	env, err := cel.NewEnv(options...)
	if err != nil {
		return nil, fmt.Errorf("create CEL environment: %w", err)
	}

	return &Evaluator{env: env, values: evalValues, limits: limits}, nil
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

	program, err := e.env.Program(ast,
		cel.CostLimit(e.limits.costLimit),
		cel.InterruptCheckFrequency(e.limits.interruptCheckFrequency),
	)
	if err != nil {
		return nil, fmt.Errorf("create CEL program for %q: %w", expr, err)
	}

	runCtx, cancel := context.WithTimeout(ctx, e.limits.timeout)
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

type aliasNode struct {
	all      bool
	children map[string]*aliasNode
}

func buildAliasTree(aliases []string) ([]string, map[string]*aliasNode, error) {
	tree := map[string]*aliasNode{}
	for _, alias := range aliases {
		parts := strings.Split(alias, ".")
		if hasEmptyPart(parts) {
			return nil, nil, fmt.Errorf("alias must not be empty")
		}
		root := parts[0]
		node := tree[root]
		if node == nil {
			node = &aliasNode{}
			tree[root] = node
		}
		node.add(parts[1:])
	}

	roots := make([]string, 0, len(tree))
	for root := range tree {
		roots = append(roots, root)
	}
	sort.Strings(roots)

	return roots, tree, nil
}

func (n *aliasNode) add(parts []string) {
	if n.all {
		return
	}
	if len(parts) == 0 {
		n.all = true
		n.children = nil
		return
	}
	if n.children == nil {
		n.children = map[string]*aliasNode{}
	}
	child := n.children[parts[0]]
	if child == nil {
		child = &aliasNode{}
		n.children[parts[0]] = child
	}
	child.add(parts[1:])
}

func hasEmptyPart(parts []string) bool {
	for _, part := range parts {
		if part == "" {
			return true
		}
	}
	return false
}

func pruneActivationValues(values map[string]any, roots []string, aliasTree map[string]*aliasNode) map[string]any {
	pruned := make(map[string]any, len(roots))
	for _, root := range roots {
		pruned[root] = pruneValue(values[root], aliasTree[root])
	}
	return pruned
}

func pruneValue(value any, node *aliasNode) any {
	if node == nil {
		return nil
	}
	if node.all {
		return value
	}

	source, ok := value.(map[string]any)
	if !ok {
		return map[string]any{}
	}

	pruned := make(map[string]any, len(node.children))
	for name, child := range node.children {
		if child.all {
			if selected, ok := source[name]; ok {
				pruned[name] = selected
			}
			continue
		}
		if selected, ok := source[name]; ok {
			pruned[name] = pruneValue(selected, child)
		}
	}
	return pruned
}
