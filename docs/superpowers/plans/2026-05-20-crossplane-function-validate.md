# Crossplane Function Validate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build v1 of `crossplane-function-validate`, a Go Crossplane composition function that evaluates declarative CEL validation rules during render and reconciliation.

**Architecture:** The function decodes a `Rules` input object, builds a stable CEL data model from the Crossplane request, resolves named required Kubernetes resources through Crossplane's required-resource protocol, evaluates rules in input order, and returns one fatal result containing all failed rule messages. The implementation keeps Kyverno generation out of scope while parsing `rejectWithKyverno` for future compatibility.

**Tech Stack:** Go, `github.com/crossplane/function-sdk-go`, `github.com/google/cel-go`, Kubernetes `runtime.Object`, Crossplane CLI render tests.

---

## Reference Material

- Design spec: `docs/superpowers/specs/2026-05-19-crossplane-function-validate-design.md`
- Crossplane Go function SDK: https://pkg.go.dev/github.com/crossplane/function-sdk-go
- Crossplane function response helpers: https://pkg.go.dev/github.com/crossplane/function-sdk-go/response
- Crossplane function proto v1 required resources: https://pkg.go.dev/github.com/crossplane/function-sdk-go/proto/v1
- CEL Go package: https://pkg.go.dev/github.com/google/cel-go/cel
- Crossplane render development runtime: https://docs.crossplane.io/latest/composition/compositions/

## File Structure

- Create `go.mod`: module and pinned dependencies.
- Create `main.go`: starts the Crossplane function server.
- Create `fn.go`: `RunFunction` orchestration and Crossplane response handling.
- Create `input/v1alpha1/rules.go`: public function input types, defaulting, and structural validation.
- Create `internal/celruntime/evaluator.go`: small wrapper around cel-go for strict aliases, bool evaluation, string evaluation, and bounded execution.
- Create `internal/model/model.go`: converts Crossplane request data into the stable CEL alias map.
- Create `internal/requiredresources/requirements.go`: resolves `nameFrom` and `namespaceFrom`, returns Crossplane required-resource selectors, and reads resolved resources back into the alias map.
- Create `internal/validation/engine.go`: evaluates `when` and `assert` expressions in rule order and aggregates user-facing failures.
- Create tests beside each package.
- Create `package/crossplane.yaml`: Crossplane Function package metadata.
- Create `Dockerfile`: runtime image build.
- Create `tests/render/`: local render fixtures using the Crossplane Development runtime.
- Create `README.md`: concise usage, local test, and rule-authoring guidance.

## Implementation Rules

- Do not implement Kyverno policy generation.
- Do not call Kubernetes directly; use Crossplane required-resource request/response fields only.
- Do not mutate desired resources.
- Keep `rejectWithKyverno` to parse, default, and enum validation.
- Use unstructured JSON-compatible values in CEL.
- Enforce `uses` by compiling each rule with only the aliases it declares.
- Return validation failures in input order.
- Keep single-object shape checks in XRD schemas; use this function for cross-resource or pipeline-aware validation.

### Task 1: Go Module, Server Skeleton, And Package Metadata

**Files:**
- Create: `go.mod`
- Create: `main.go`
- Create: `fn.go`
- Create: `package/crossplane.yaml`
- Create: `Dockerfile`
- Create: `.gitignore`
- Test: `go test ./...`

- [ ] **Step 1: Create module and dependency baseline**

Create `go.mod`:

```go
module github.com/io41/crossplane-function-validate

go 1.25

require (
	github.com/crossplane/function-sdk-go v0.6.2
	github.com/google/cel-go v0.28.0
	k8s.io/apimachinery v0.35.1
)
```

- [ ] **Step 2: Create the function server entrypoint**

Create `main.go`:

```go
package main

import (
	"flag"
	"log"

	"github.com/crossplane/function-sdk-go"
)

func main() {
	insecure := flag.Bool("insecure", false, "Run without mTLS. Use only for local crossplane render development.")
	flag.Parse()

	opts := []function.ServeOption{}
	if *insecure {
		opts = append(opts, function.Insecure(true))
	}

	if err := function.Serve(&Function{}, opts...); err != nil {
		log.Fatalf("cannot serve function: %v", err)
	}
}
```

- [ ] **Step 3: Create a no-op function skeleton**

Create `fn.go`:

```go
package main

import (
	"context"

	"github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/response"
)

type Function struct {
	v1.UnimplementedFunctionRunnerServiceServer
}

func (f *Function) RunFunction(ctx context.Context, req *v1.RunFunctionRequest) (*v1.RunFunctionResponse, error) {
	return response.To(req, response.DefaultTTL), nil
}
```

- [ ] **Step 4: Add package metadata**

Create `package/crossplane.yaml`:

```yaml
apiVersion: meta.pkg.crossplane.io/v1
kind: Function
metadata:
  name: function-validate
  annotations:
    meta.crossplane.io/maintainer: Tim Kersten
    meta.crossplane.io/source: github.com/io41/crossplane-function-validate
    meta.crossplane.io/license: Apache-2.0
spec:
  capabilities:
    - composition
```

- [ ] **Step 5: Add Dockerfile**

Create `Dockerfile`:

```Dockerfile
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /function .

FROM gcr.io/distroless/static:nonroot
COPY --from=build /function /function
USER 65532:65532
ENTRYPOINT ["/function"]
```

- [ ] **Step 6: Add ignore file**

Create `.gitignore`:

```gitignore
/bin/
/dist/
*.test
coverage.out
```

- [ ] **Step 7: Download modules and verify**

Run:

```bash
go mod tidy
go test ./...
```

Expected: `go test ./...` exits 0. It may report no test files at this point.

- [ ] **Step 8: Commit scaffold**

```bash
git add .gitignore Dockerfile fn.go go.mod go.sum main.go package/crossplane.yaml
git commit -m "feat: scaffold validation function"
```

### Task 2: Input API Types, Defaulting, And Structural Validation

**Files:**
- Create: `input/v1alpha1/rules.go`
- Create: `input/v1alpha1/rules_test.go`

- [ ] **Step 1: Write failing input tests**

Create `input/v1alpha1/rules_test.go`:

```go
package v1alpha1

import (
	"strings"
	"testing"
)

func TestRulesDefaultRejectWithKyverno(t *testing.T) {
	r := &Rules{Spec: RulesSpec{Rules: []Rule{{ID: "r1", Uses: []string{"xr"}, Assert: "true", Message: "ok"}}}}
	r.Default()
	if got := r.Spec.Rules[0].RejectWithKyverno; got != RejectWithKyvernoAuto {
		t.Fatalf("default rejectWithKyverno = %q, want %q", got, RejectWithKyvernoAuto)
	}
}

func TestRulesValidateRejectsDuplicateIDs(t *testing.T) {
	r := &Rules{Spec: RulesSpec{Rules: []Rule{
		{ID: "same", Uses: []string{"xr"}, Assert: "true", Message: "one"},
		{ID: "same", Uses: []string{"xr"}, Assert: "true", Message: "two"},
	}}}
	r.Default()
	err := r.Validate()
	if err == nil || !strings.Contains(err.Error(), `duplicate rule id "same"`) {
		t.Fatalf("Validate() error = %v, want duplicate rule id", err)
	}
}

func TestRulesValidateRejectsInvalidKyvernoMode(t *testing.T) {
	r := &Rules{Spec: RulesSpec{Rules: []Rule{{
		ID: "r1", Uses: []string{"xr"}, RejectWithKyverno: "Soon", Assert: "true", Message: "ok",
	}}}}
	err := r.Validate()
	if err == nil || !strings.Contains(err.Error(), `rejectWithKyverno`) {
		t.Fatalf("Validate() error = %v, want rejectWithKyverno error", err)
	}
}

func TestRulesValidateRejectsRequiredInputWithoutName(t *testing.T) {
	r := &Rules{Spec: RulesSpec{Inputs: Inputs{Required: map[string]RequiredResource{
		"namespace": {APIVersion: "example.org/v1", Kind: "XNamespace"},
	}}, Rules: []Rule{{ID: "r1", Uses: []string{"required.namespace"}, Assert: "true", Message: "ok"}}}}
	err := r.Validate()
	if err == nil || !strings.Contains(err.Error(), `required input "namespace" must set name or nameFrom`) {
		t.Fatalf("Validate() error = %v, want required input name error", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./input/v1alpha1
```

Expected: FAIL because `Rules`, `RejectWithKyvernoAuto`, and validation methods are not defined.

- [ ] **Step 3: Implement input API types**

Create `input/v1alpha1/rules.go`:

```go
package v1alpha1

import (
	"fmt"
	"sort"

	"k8s.io/apimachinery/pkg/runtime"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	RejectWithKyvernoAuto   = "Auto"
	RejectWithKyvernoAlways = "Always"
	RejectWithKyvernoNever  = "Never"
)

type Rules struct {
	metav1.TypeMeta `json:",inline"`
	Spec            RulesSpec `json:"spec,omitempty"`
}

type RulesSpec struct {
	Inputs Inputs `json:"inputs,omitempty"`
	Rules  []Rule `json:"rules,omitempty"`
}

type Inputs struct {
	Required map[string]RequiredResource `json:"required,omitempty"`
}

type RequiredResource struct {
	APIVersion    string `json:"apiVersion,omitempty"`
	Kind          string `json:"kind,omitempty"`
	Name          string `json:"name,omitempty"`
	Namespace     string `json:"namespace,omitempty"`
	NameFrom      string `json:"nameFrom,omitempty"`
	NamespaceFrom string `json:"namespaceFrom,omitempty"`
}

type Rule struct {
	ID                 string   `json:"id,omitempty"`
	Description        string   `json:"description,omitempty"`
	Uses               []string `json:"uses,omitempty"`
	RejectWithKyverno  string   `json:"rejectWithKyverno,omitempty"`
	When               string   `json:"when,omitempty"`
	Assert             string   `json:"assert,omitempty"`
	Message            string   `json:"message,omitempty"`
}

func (r *Rules) DeepCopyObject() runtime.Object {
	if r == nil {
		return nil
	}
	out := *r
	out.Spec.Rules = append([]Rule(nil), r.Spec.Rules...)
	if r.Spec.Inputs.Required != nil {
		out.Spec.Inputs.Required = make(map[string]RequiredResource, len(r.Spec.Inputs.Required))
		for k, v := range r.Spec.Inputs.Required {
			out.Spec.Inputs.Required[k] = v
		}
	}
	return &out
}

func (r *Rules) Default() {
	for i := range r.Spec.Rules {
		if r.Spec.Rules[i].RejectWithKyverno == "" {
			r.Spec.Rules[i].RejectWithKyverno = RejectWithKyvernoAuto
		}
	}
}

func (r *Rules) Validate() error {
	if r == nil {
		return fmt.Errorf("input Rules is nil")
	}
	for name, rr := range r.Spec.Inputs.Required {
		if rr.APIVersion == "" {
			return fmt.Errorf("required input %q must set apiVersion", name)
		}
		if rr.Kind == "" {
			return fmt.Errorf("required input %q must set kind", name)
		}
		if rr.Name == "" && rr.NameFrom == "" {
			return fmt.Errorf("required input %q must set name or nameFrom", name)
		}
	}

	seen := map[string]struct{}{}
	for i, rule := range r.Spec.Rules {
		if rule.ID == "" {
			return fmt.Errorf("rule at index %d must set id", i)
		}
		if _, ok := seen[rule.ID]; ok {
			return fmt.Errorf("duplicate rule id %q", rule.ID)
		}
		seen[rule.ID] = struct{}{}
		if len(rule.Uses) == 0 {
			return fmt.Errorf("rule %q must set uses", rule.ID)
		}
		if rule.Assert == "" {
			return fmt.Errorf("rule %q must set assert", rule.ID)
		}
		if rule.Message == "" {
			return fmt.Errorf("rule %q must set message", rule.ID)
		}
		switch rule.RejectWithKyverno {
		case RejectWithKyvernoAuto, RejectWithKyvernoAlways, RejectWithKyvernoNever:
		default:
			return fmt.Errorf("rule %q has invalid rejectWithKyverno %q", rule.ID, rule.RejectWithKyverno)
		}
	}
	return nil
}

func (r *Rules) RequiredInputNames() []string {
	names := make([]string, 0, len(r.Spec.Inputs.Required))
	for name := range r.Spec.Inputs.Required {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
```

- [ ] **Step 4: Run tests**

Run:

```bash
go test ./input/v1alpha1
```

Expected: PASS.

- [ ] **Step 5: Commit input API**

```bash
git add input/v1alpha1/rules.go input/v1alpha1/rules_test.go
git commit -m "feat: define rules input api"
```

### Task 3: CEL Runtime Wrapper

**Files:**
- Create: `internal/celruntime/evaluator.go`
- Create: `internal/celruntime/evaluator_test.go`

- [ ] **Step 1: Write failing CEL runtime tests**

Create `internal/celruntime/evaluator_test.go`:

```go
package celruntime

import (
	"context"
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/celruntime
```

Expected: FAIL because package implementation is missing.

- [ ] **Step 3: Implement CEL wrapper**

Create `internal/celruntime/evaluator.go`:

```go
package celruntime

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types/ref"
)

const defaultCostLimit uint64 = 100000

type Evaluator struct {
	env    *cel.Env
	values map[string]any
}

func New(aliases []string, values map[string]any) (*Evaluator, error) {
	roots := aliasRoots(aliases)
	options := make([]cel.EnvOption, 0, len(roots))
	evalValues := make(map[string]any, len(roots))
	for _, root := range roots {
		options = append(options, cel.Variable(root, cel.DynType))
		evalValues[root] = values[root]
	}
	env, err := cel.NewEnv(options...)
	if err != nil {
		return nil, err
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
	ast, issues := e.env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}
	program, err := e.env.Program(ast, cel.CostLimit(defaultCostLimit))
	if err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	out, _, err := program.ContextEval(runCtx, e.values)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func aliasRoots(aliases []string) []string {
	seen := map[string]struct{}{}
	for _, alias := range aliases {
		seen[aliasRoot(alias)] = struct{}{}
	}
	roots := make([]string, 0, len(seen))
	for root := range seen {
		roots = append(roots, root)
	}
	sort.Strings(roots)
	return roots
}

func aliasRoot(alias string) string {
	for i, r := range alias {
		if r == '.' {
			return alias[:i]
		}
	}
	return alias
}
```

- [ ] **Step 4: Run CEL tests**

Run:

```bash
go test ./internal/celruntime
```

Expected: PASS.

- [ ] **Step 5: Commit CEL runtime**

```bash
git add internal/celruntime/evaluator.go internal/celruntime/evaluator_test.go
git commit -m "feat: add bounded cel evaluator"
```

### Task 4: Request Data Model

**Files:**
- Create: `internal/model/model.go`
- Create: `internal/model/model_test.go`

- [ ] **Step 1: Write failing model tests**

Create `internal/model/model_test.go`:

```go
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
			"kind": "XR",
			"spec": map[string]any{"environment": "dev"},
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
			"kind": "XR",
			"spec": map[string]any{"environment": "dev"},
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/model
```

Expected: FAIL because `Build` is missing.

- [ ] **Step 3: Implement model builder**

Create `internal/model/model.go`:

```go
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
		"claim": nil,
		"context": structToMap(req.GetContext()),
		"observed": stateToMap(req.GetObserved()),
		"desired": stateToMap(req.GetDesired()),
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
```

- [ ] **Step 4: Run tests**

Run:

```bash
go test ./internal/model
```

Expected: PASS.

- [ ] **Step 5: Commit model builder**

```bash
git add internal/model/model.go internal/model/model_test.go
git commit -m "feat: expose crossplane request model"
```

### Task 5: Required Resource Protocol

**Files:**
- Create: `internal/requiredresources/requirements.go`
- Create: `internal/requiredresources/requirements_test.go`

- [ ] **Step 1: Write failing requirements tests**

Create `internal/requiredresources/requirements_test.go`:

```go
package requiredresources

import (
	"context"
	"testing"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/io41/crossplane-function-validate/input/v1alpha1"
)

func TestBuildRequirementsResolvesNameFrom(t *testing.T) {
	rules := &v1alpha1.Rules{Spec: v1alpha1.RulesSpec{Inputs: v1alpha1.Inputs{Required: map[string]v1alpha1.RequiredResource{
		"namespace": {
			APIVersion: "example.org/v1",
			Kind: "XNamespace",
			NameFrom: "xr.spec.serviceBusRef.name",
			NamespaceFrom: "xr.spec.serviceBusRef.namespace",
		},
	}}}}
	aliases := map[string]any{"xr": map[string]any{"spec": map[string]any{"serviceBusRef": map[string]any{"name": "bus", "namespace": "platform"}}}}
	reqs, err := BuildRequirements(context.Background(), rules, aliases)
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
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/requiredresources
```

Expected: FAIL because the package implementation is missing.

- [ ] **Step 3: Implement required-resource helpers**

Create `internal/requiredresources/requirements.go`:

```go
package requiredresources

import (
	"context"
	"fmt"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/io41/crossplane-function-validate/input/v1alpha1"
	"github.com/io41/crossplane-function-validate/internal/celruntime"
)

func BuildRequirements(ctx context.Context, rules *v1alpha1.Rules, aliases map[string]any) (map[string]*fnv1.ResourceSelector, error) {
	out := map[string]*fnv1.ResourceSelector{}
	for _, name := range rules.RequiredInputNames() {
		spec := rules.Spec.Inputs.Required[name]
		matchName := spec.Name
		namespace := spec.Namespace
		ev, err := celruntime.New([]string{"xr", "claim", "context"}, aliases)
		if err != nil {
			return nil, err
		}
		if spec.NameFrom != "" {
			matchName, err = ev.EvalString(ctx, spec.NameFrom)
			if err != nil {
				return nil, fmt.Errorf("required input %q nameFrom: %w", name, err)
			}
		}
		if spec.NamespaceFrom != "" {
			namespace, err = ev.EvalString(ctx, spec.NamespaceFrom)
			if err != nil {
				return nil, fmt.Errorf("required input %q namespaceFrom: %w", name, err)
			}
		}
		selector := &fnv1.ResourceSelector{
			ApiVersion: spec.APIVersion,
			Kind: spec.Kind,
			Match: &fnv1.ResourceSelector_MatchName{MatchName: matchName},
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
	if !ok {
		return false, fmt.Errorf("required alias is missing or not an object")
	}
	allResolved := true
	for _, name := range names {
		resources, ok := req.GetRequiredResources()[name]
		if !ok {
			allResolved = false
			continue
		}
		if len(resources.GetItems()) == 0 {
			required[name] = nil
			continue
		}
		required[name] = resources.GetItems()[0].GetResource().AsMap()
	}
	return allResolved, nil
}
```

- [ ] **Step 4: Run tests**

Run:

```bash
go test ./internal/requiredresources
```

Expected: PASS.

- [ ] **Step 5: Commit required resources**

```bash
git add internal/requiredresources/requirements.go internal/requiredresources/requirements_test.go
git commit -m "feat: resolve required resources"
```

### Task 6: Validation Engine

**Files:**
- Create: `internal/validation/engine.go`
- Create: `internal/validation/engine_test.go`

- [ ] **Step 1: Write failing validation tests**

Create `internal/validation/engine_test.go`:

```go
package validation

import (
	"context"
	"testing"

	"github.com/io41/crossplane-function-validate/input/v1alpha1"
)

func TestEvaluateReturnsFailuresInRuleOrder(t *testing.T) {
	rules := &v1alpha1.Rules{Spec: v1alpha1.RulesSpec{Rules: []v1alpha1.Rule{
		{ID: "first", Uses: []string{"xr"}, Assert: "false", Message: "first failed"},
		{ID: "second", Uses: []string{"xr"}, Assert: "false", Message: "second failed"},
	}}}
	rules.Default()
	out, err := Evaluate(context.Background(), rules, map[string]any{"xr": map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Failures) != 2 {
		t.Fatalf("failures = %d, want 2", len(out.Failures))
	}
	if out.Failures[0].Message != "first failed" || out.Failures[1].Message != "second failed" {
		t.Fatalf("failure order = %#v", out.Failures)
	}
}

func TestEvaluateSkipsWhenFalse(t *testing.T) {
	rules := &v1alpha1.Rules{Spec: v1alpha1.RulesSpec{Rules: []v1alpha1.Rule{{
		ID: "skip", Uses: []string{"xr"}, When: "false", Assert: "false", Message: "must not appear",
	}}}}
	rules.Default()
	out, err := Evaluate(context.Background(), rules, map[string]any{"xr": map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Failures) != 0 {
		t.Fatalf("failures = %#v, want none", out.Failures)
	}
}

func TestEvaluateRejectsUndeclaredAlias(t *testing.T) {
	rules := &v1alpha1.Rules{Spec: v1alpha1.RulesSpec{Rules: []v1alpha1.Rule{{
		ID: "bad", Uses: []string{"xr"}, Assert: "required.namespace != null", Message: "bad",
	}}}}
	rules.Default()
	_, err := Evaluate(context.Background(), rules, map[string]any{"xr": map[string]any{}, "required": map[string]any{"namespace": nil}})
	if err == nil {
		t.Fatal("Evaluate() error = nil, want undeclared alias error")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/validation
```

Expected: FAIL because `Evaluate` is missing.

- [ ] **Step 3: Implement validation engine**

Create `internal/validation/engine.go`:

```go
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
		values := valuesForUses(rule.Uses, aliases)
		ev, err := celruntime.New(rule.Uses, values)
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

func valuesForUses(uses []string, aliases map[string]any) map[string]any {
	out := map[string]any{}
	for _, use := range uses {
		root := aliasRoot(use)
		if value, ok := aliases[root]; ok {
			out[root] = value
		}
	}
	return out
}

func aliasRoot(alias string) string {
	for i, r := range alias {
		if r == '.' {
			return alias[:i]
		}
	}
	return alias
}
```

- [ ] **Step 4: Run tests**

Run:

```bash
go test ./internal/validation
```

Expected: PASS.

- [ ] **Step 5: Commit validation engine**

```bash
git add internal/validation/engine.go internal/validation/engine_test.go
git commit -m "feat: evaluate validation rules"
```

### Task 7: Wire Crossplane RunFunction

**Files:**
- Modify: `fn.go`
- Create: `fn_test.go`

- [ ] **Step 1: Write failing function tests**

Create `fn_test.go`:

```go
package main

import (
	"context"
	"strings"
	"testing"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestRunFunctionReturnsRequiredResourceSelectors(t *testing.T) {
	input := mustStruct(t, map[string]any{
		"apiVersion": "validate.fn.crossplane.io/v1alpha1",
		"kind": "Rules",
		"spec": map[string]any{
			"inputs": map[string]any{"required": map[string]any{"namespace": map[string]any{
				"apiVersion": "example.org/v1",
				"kind": "XNamespace",
				"nameFrom": "xr.spec.ref.name",
				"namespaceFrom": "xr.spec.ref.namespace",
			}}},
			"rules": []any{map[string]any{"id": "exists", "uses": []any{"required.namespace"}, "assert": "required.namespace != null", "message": "namespace missing"}},
		},
	})
	req := &fnv1.RunFunctionRequest{
		Input: input,
		Observed: &fnv1.State{Composite: &fnv1.Resource{Resource: mustStruct(t, map[string]any{"spec": map[string]any{"ref": map[string]any{"name": "bus", "namespace": "platform"}}})}},
	}
	rsp, err := (&Function{}).RunFunction(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	got := rsp.GetRequirements().GetResources()["namespace"]
	if got.GetMatchName() != "bus" || got.GetNamespace() != "platform" {
		t.Fatalf("selector = %#v, want bus/platform", got)
	}
}

func TestRunFunctionAggregatesValidationFailures(t *testing.T) {
	input := mustStruct(t, map[string]any{
		"apiVersion": "validate.fn.crossplane.io/v1alpha1",
		"kind": "Rules",
		"spec": map[string]any{"rules": []any{
			map[string]any{"id": "one", "uses": []any{"xr"}, "assert": "false", "message": "first failed"},
			map[string]any{"id": "two", "uses": []any{"xr"}, "assert": "false", "message": "second failed"},
		}},
	})
	req := &fnv1.RunFunctionRequest{
		Input: input,
		Observed: &fnv1.State{Composite: &fnv1.Resource{Resource: mustStruct(t, map[string]any{"spec": map[string]any{}})}},
	}
	rsp, err := (&Function{}).RunFunction(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(rsp.GetResults()) != 1 {
		t.Fatalf("results = %d, want 1", len(rsp.GetResults()))
	}
	msg := rsp.GetResults()[0].GetMessage()
	if !strings.Contains(msg, "first failed") || !strings.Contains(msg, "second failed") {
		t.Fatalf("fatal message = %q, want both failures", msg)
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./...
```

Expected: FAIL because `RunFunction` is still no-op.

- [ ] **Step 3: Implement function orchestration**

Modify `fn.go`:

```go
package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/response"
	"github.com/io41/crossplane-function-validate/input/v1alpha1"
	"github.com/io41/crossplane-function-validate/internal/model"
	"github.com/io41/crossplane-function-validate/internal/requiredresources"
	"github.com/io41/crossplane-function-validate/internal/validation"
)

type Function struct {
	v1.UnimplementedFunctionRunnerServiceServer
}

func (f *Function) RunFunction(ctx context.Context, req *v1.RunFunctionRequest) (*v1.RunFunctionResponse, error) {
	rsp := response.To(req, response.DefaultTTL)

	var rules v1alpha1.Rules
	if err := request.GetInput(req, &rules); err != nil {
		response.Fatal(rsp, fmt.Errorf("invalid Rules input: %w", err))
		return rsp, nil
	}
	rules.Default()
	if err := rules.Validate(); err != nil {
		response.Fatal(rsp, fmt.Errorf("invalid Rules input: %w", err))
		return rsp, nil
	}

	m, err := model.Build(req)
	if err != nil {
		response.Fatal(rsp, err)
		return rsp, nil
	}

	reqs, err := requiredresources.BuildRequirements(ctx, &rules, m.Aliases)
	if err != nil {
		response.Fatal(rsp, err)
		return rsp, nil
	}
	if len(reqs) > 0 {
		rsp.Requirements = &v1.Requirements{Resources: reqs}
	}

	resolved, err := requiredresources.ApplyResolvedResources(req, m.Aliases, rules.RequiredInputNames())
	if err != nil {
		response.Fatal(rsp, err)
		return rsp, nil
	}
	if len(reqs) > 0 && !resolved {
		return rsp, nil
	}

	out, err := validation.Evaluate(ctx, &rules, m.Aliases)
	if err != nil {
		response.Fatal(rsp, err)
		return rsp, nil
	}
	if len(out.Failures) > 0 {
		response.Fatal(rsp, fmt.Errorf(formatFailures(out.Failures)))
		return rsp, nil
	}
	return rsp, nil
}

func formatFailures(failures []validation.Failure) string {
	lines := make([]string, 0, len(failures))
	for _, failure := range failures {
		lines = append(lines, "- "+failure.Message)
	}
	return "validation failed:\n" + strings.Join(lines, "\n")
}
```

- [ ] **Step 4: Run all tests**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 5: Commit function wiring**

```bash
git add fn.go fn_test.go
git commit -m "feat: wire validation function"
```

### Task 8: Render Integration Fixtures

**Files:**
- Create: `tests/render/xr.yaml`
- Create: `tests/render/composition.yaml`
- Create: `tests/render/functions.yaml`
- Create: `tests/render/extra-resources.yaml`
- Create: `tests/render/expect-render-failure.sh`

- [ ] **Step 1: Create render XR fixture**

Create `tests/render/xr.yaml`:

```yaml
apiVersion: example.org/v1alpha1
kind: XTopic
metadata:
  name: orders
spec:
  environment: int
  serviceBusRef:
    name: bus
    namespace: platform
```

- [ ] **Step 2: Create composition fixture**

Create `tests/render/composition.yaml`:

```yaml
apiVersion: apiextensions.crossplane.io/v1
kind: Composition
metadata:
  name: xtopic.example.org
spec:
  compositeTypeRef:
    apiVersion: example.org/v1alpha1
    kind: XTopic
  mode: Pipeline
  pipeline:
    - step: validate
      functionRef:
        name: function-validate
      input:
        apiVersion: validate.fn.crossplane.io/v1alpha1
        kind: Rules
        spec:
          inputs:
            required:
              namespace:
                apiVersion: example.org/v1alpha1
                kind: XNamespace
                nameFrom: xr.spec.serviceBusRef.name
                namespaceFrom: xr.spec.serviceBusRef.namespace
          rules:
            - id: namespace-exists
              uses:
                - required.namespace
              assert: required.namespace != null
              message: Referenced Service Bus namespace does not exist.
            - id: namespace-environment-matches-topic
              uses:
                - xr
                - required.namespace
              when: required.namespace != null && has(required.namespace.spec.environment)
              assert: required.namespace.spec.environment == xr.spec.environment
              message: Referenced Service Bus namespace does not allow this environment.
```

- [ ] **Step 3: Create development runtime function fixture**

Create `tests/render/functions.yaml`:

```yaml
apiVersion: pkg.crossplane.io/v1
kind: Function
metadata:
  name: function-validate
  annotations:
    render.crossplane.io/runtime: Development
spec:
  package: ghcr.io/io41/crossplane-function-validate:v0.0.0
```

- [ ] **Step 4: Create extra resources fixture**

Create `tests/render/extra-resources.yaml`:

```yaml
apiVersion: example.org/v1alpha1
kind: XNamespace
metadata:
  name: bus
  namespace: platform
spec:
  environment: dev
```

- [ ] **Step 5: Create render failure helper**

Create `tests/render/expect-render-failure.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

set +e
crossplane render tests/render/xr.yaml tests/render/composition.yaml tests/render/functions.yaml \
  --extra-resources tests/render/extra-resources.yaml >"$tmp" 2>&1
status=$?
set -e

if [[ "$status" -eq 0 ]]; then
  echo "crossplane render succeeded, expected failure" >&2
  cat "$tmp" >&2
  exit 1
fi

if ! grep -q "Referenced Service Bus namespace does not allow this environment." "$tmp"; then
  echo "crossplane render failed without expected validation message" >&2
  cat "$tmp" >&2
  exit 1
fi
```

- [ ] **Step 6: Make helper executable**

Run:

```bash
chmod +x tests/render/expect-render-failure.sh
```

- [ ] **Step 7: Run function locally and render**

Run terminal 1:

```bash
go run . --insecure
```

Expected: command keeps running and listens on `localhost:9443`.

Run terminal 2:

```bash
tests/render/expect-render-failure.sh
```

Expected: command exits 0 after confirming the validation failure message.

- [ ] **Step 8: Commit render fixtures**

```bash
git add tests/render
git commit -m "test: add render validation fixture"
```

### Task 9: README And Final Verification

**Files:**
- Create: `README.md`

- [ ] **Step 1: Create README**

Create `README.md`:

````markdown
# crossplane-function-validate

`crossplane-function-validate` is a Crossplane composition function that evaluates declarative CEL validation rules during render and reconciliation.

Use XRD schema and XRD CEL validation for simple single-object shape checks. Use this function when validation needs referenced Kubernetes resources, Crossplane pipeline state, or reusable rule logic shared across compositions.

## Example

```yaml
apiVersion: validate.fn.crossplane.io/v1alpha1
kind: Rules
spec:
  inputs:
    required:
      namespace:
        apiVersion: example.org/v1alpha1
        kind: XNamespace
        nameFrom: xr.spec.serviceBusRef.name
        namespaceFrom: xr.spec.serviceBusRef.namespace
  rules:
    - id: namespace-exists
      uses:
        - required.namespace
      assert: required.namespace != null
      message: Referenced Service Bus namespace does not exist.
    - id: namespace-environment-matches-topic
      uses:
        - xr
        - required.namespace
      rejectWithKyverno: Auto
      when: required.namespace != null && has(required.namespace.spec.environment)
      assert: required.namespace.spec.environment == xr.spec.environment
      message: Referenced Service Bus namespace does not allow this environment.
```

## Local Test

```bash
go test ./...
```

For render testing, run the function in one terminal:

```bash
go run . --insecure
```

Then run:

```bash
tests/render/expect-render-failure.sh
```

## v1 Scope

- CEL rule evaluation.
- Named Crossplane required resources.
- Human-readable validation failures.
- `rejectWithKyverno` parsing for future generated Kyverno rejection.

v1 does not generate Kyverno policies, call cloud APIs, call arbitrary HTTP APIs, query remote clusters, or mutate desired resources.
````

- [ ] **Step 2: Run final verification**

Run:

```bash
go test ./...
git diff --check
```

Expected: both commands exit 0.

- [ ] **Step 3: Run render verification**

Start the function:

```bash
go run . --insecure
```

In another terminal, run:

```bash
tests/render/expect-render-failure.sh
```

Expected: helper exits 0.

- [ ] **Step 4: Commit README**

```bash
git add README.md
git commit -m "docs: document validation function"
```

## Plan Self-Review Checklist

- Spec coverage: Tasks cover input API, strict `uses`, unstructured CEL values, absent claim behavior, required-resource selectors, not-found-as-null behavior, ordered evaluation, one aggregated fatal result, render fixture behavior, and v1 Kyverno non-generation.
- Test coverage: Unit tests cover each package boundary, and render tests cover the Crossplane CLI path.
- Type consistency: Public input types live in `input/v1alpha1`; runtime packages consume those types consistently.
- Scope control: Kyverno generation, CRD schema typing, remote clusters, HTTP/cloud APIs, and desired-resource mutation stay out of v1.
