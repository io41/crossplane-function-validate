package celruntime

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestEvalBoolWithDeclaredAlias(t *testing.T) {
	ev, err := New([]string{"xr"}, map[string]any{"xr": map[string]any{"spec": map[string]any{"environment": "dev"}}})
	if err != nil {
		t.Fatal(err)
	}

	got, err := ev.EvalBool(context.Background(), `xr.spec.environment == "dev"`)
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Fatal("EvalBool() = false, want true")
	}
}

func TestEvalBoolRejectsUndeclaredAlias(t *testing.T) {
	ev, err := New([]string{"xr"}, map[string]any{"xr": map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}

	_, err = ev.EvalBool(context.Background(), `required.namespace != null`)
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("EvalBool() error = %v, want undeclared alias", err)
	}
}

func TestEvalStringRequiresStringResult(t *testing.T) {
	ev, err := New([]string{"xr"}, map[string]any{"xr": map[string]any{"spec": map[string]any{"replicas": 2}}})
	if err != nil {
		t.Fatal(err)
	}

	_, err = ev.EvalString(context.Background(), `xr.spec.replicas`)
	if err == nil || !strings.Contains(err.Error(), "must evaluate to string") {
		t.Fatalf("EvalString() error = %v, want string type error", err)
	}
}

func TestEvalBoolHonorsContextCancellation(t *testing.T) {
	ev, err := newWithLimits([]string{"xs"}, map[string]any{"xs": []int64{1, 2, 3}}, evaluatorLimits{
		costLimit:               defaultCostLimit,
		interruptCheckFrequency: 1,
		timeout:                 defaultTimeout,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = ev.EvalBool(ctx, `xs.exists(x, x == -1)`)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("EvalBool() error = %v, want context.Canceled", err)
	}
}

func TestEvalBoolEnforcesCostLimit(t *testing.T) {
	ev, err := newWithLimits([]string{"xr"}, map[string]any{"xr": map[string]any{"spec": map[string]any{"environment": "dev"}}}, evaluatorLimits{
		costLimit:               0,
		interruptCheckFrequency: defaultInterruptCheckFrequency,
		timeout:                 defaultTimeout,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = ev.EvalBool(context.Background(), `xr.spec.environment == "dev"`)
	if err == nil || !strings.Contains(err.Error(), "cost limit") {
		t.Fatalf("EvalBool() error = %v, want cost limit error", err)
	}
}
