# crossplane-function-validate Design

## Summary

`crossplane-function-validate` is a Go-based Crossplane composition function for render-time and reconcile-time validation. It lets composition authors move validation out of ad hoc template logic and into declarative CEL rules with clear, user-owned messages.

The first version validates only data available through the Crossplane function pipeline: the composite resource, pipeline context, observed and desired resources, and named required Kubernetes resources. It does not call arbitrary HTTP services, cloud APIs, or remote clusters, and it does not try to type-check rules against CRD schemas.

The design also leaves room for a future Kyverno generator. Rules can opt into, out of, or defer generated Kyverno rejection with `rejectWithKyverno`, so the same validation source can eventually support both Crossplane render failures and Kubernetes admission rejection where technically possible.

## Goals

- Provide a reusable Crossplane function for validation-only pipeline steps.
- Use CEL as the rule language.
- Expose a stable, purpose-built CEL data model instead of the raw Crossplane SDK request.
- Support named required Kubernetes resources so rules can validate referenced objects.
- Produce human-readable validation messages during `crossplane render` and reconciliation.
- Evaluate all applicable rules and report all validation failures in a stable order.
- Treat missing referenced resources as validation data, not automatically as developer errors.
- Keep v1 narrow enough to be simple to test and operate.
- Preserve a clean future path for deriving Kyverno policies from eligible rules.

## Non-Goals

- Generate Kyverno policies in v1.
- Query arbitrary HTTP endpoints, cloud provider APIs, or remote clusters in v1.
- Mutate desired resources.
- Replace XRD schema validation for simple static checks.
- Expose claim resources in v1; Crossplane's function request does not provide the claim object directly.
- Expose the raw Crossplane `RunFunctionRequest` as the public CEL API.
- Type-check CEL rules against XRD or CRD OpenAPI schemas in v1.
- Become a general policy engine.

## Architecture

The function runs as a normal Crossplane composition function step. It should usually run early in the pipeline, before later steps assume referenced resources are compatible.

The function accepts a `Rules` input document, resolves the declared inputs, compiles and evaluates CEL rules, and returns one fatal Crossplane function result when validation fails. That result contains all failed rule messages in a stable order. The function does not create, update, or delete desired resources.

The CEL environment exposes stable aliases:

- `xr`: the composite resource.
- `context`: Crossplane pipeline context.
- `observed`: observed composed resources.
- `desired`: desired composed resources.
- `required`: named required Kubernetes resources.

The v1 CEL environment uses unstructured JSON-compatible values. This keeps the first implementation independent of CRD schemas and works well for unknown composite and required-resource types. Field mistakes are caught through strict alias checks, CEL compilation where possible, and runtime developer errors. Schema-aware CEL typing may be added later.

Named required resources must be resolved through Crossplane's required resource mechanism, not a direct Kubernetes client. That keeps local `crossplane render` behavior aligned with in-cluster reconciliation.

### Required Resource Protocol

Crossplane required resources use an iterative protocol. On the first pass, the function evaluates each required resource selector from data that is already present in the request, then returns Crossplane resource requirements. Crossplane fetches those resources and invokes the function again with the resources available as extra resources.

The function must return the same requirements on later passes once the input data is stable. If the required-resource selectors do not stabilize, Crossplane will eventually stop the function pipeline rather than loop forever.

For v1:

- `nameFrom` and `namespaceFrom` are evaluated before the referenced resource is available.
- Selector expressions may use `xr` and initial pipeline `context`.
- Selector expressions must not depend on `required`, `observed`, or `desired`.
- The function translates the resolved literal name and namespace into Crossplane required-resource selectors.
- A declared required-resource alias is always present in CEL rule evaluation.
- If the Kubernetes resource is not found, the alias value is `null`.

Missing referenced resources are usually user-facing validation failures. For example, one rule can assert that `required.namespace != null`, and a later rule can check fields on that namespace only when it exists.

## Input API

The function input is a Kubernetes-style document embedded in a function step.

```yaml
apiVersion: validate.fn.crossplane.io/v1alpha1
kind: Rules
spec:
  inputs:
    required:
      namespace:
        apiVersion: asb.platform.example.org/v1alpha1
        kind: XNamespace
        nameFrom: xr.spec.serviceBusRef.name
        namespaceFrom: xr.spec.serviceBusRef.namespace
  rules:
    - id: namespace-exists
      description: Referenced Service Bus namespace must exist before topic validation can continue.
      uses:
        - required.namespace
      rejectWithKyverno: Auto
      assert: required.namespace != null
      message: Referenced Service Bus namespace does not exist.
    - id: namespace-environment-matches-topic
      description: Referenced Service Bus namespace must allow this topic environment.
      uses:
        - xr
        - required.namespace
      rejectWithKyverno: Auto
      when: required.namespace != null && has(required.namespace.spec.environment)
      assert: required.namespace.spec.environment == xr.spec.environment
      message: Referenced Service Bus namespace does not allow this environment.
```

### Inputs

`spec.inputs.required` declares named Kubernetes resources that rules can reference under `required.<name>`.

Each required resource supports:

- `apiVersion`: resource API version.
- `kind`: resource kind.
- `name`: literal name, for static references.
- `namespace`: literal namespace, for static namespaced references.
- `nameFrom`: CEL expression that resolves the name.
- `namespaceFrom`: CEL expression that resolves the namespace.

At least one of `name` or `nameFrom` must be set. At least one of `namespace` or `namespaceFrom` must be set in v1, so name-based required-resource selectors do not accidentally target cluster-scoped resources. Selector expressions must resolve to strings.

### Rules

Each rule supports:

- `id`: stable identifier used in logs, test assertions, and diagnostics.
- `description`: developer-facing explanation of the rule.
- `uses`: explicit aliases the rule depends on.
- `rejectWithKyverno`: future Kyverno generation preference. Defaults to `Auto`.
- `when`: optional CEL precondition. If false, the rule is skipped.
- `assert`: CEL expression that must evaluate to true.
- `message`: user-facing validation failure message.

Rule ids must be unique within a `Rules` document.

`uses` is strict in v1. A rule may reference only aliases listed in `uses`; an undeclared alias reference is a fatal input error that names the rule id and alias. The implementation should enforce this by exposing only the listed aliases to each rule's CEL environment. This keeps future Kyverno generation honest and prevents review drift between declared dependencies and actual CEL expressions.

Rule messages should tell users what is wrong or what action to take. They should not expose implementation formulas unless that formula is the actual user contract.

## Kyverno Future Path

`rejectWithKyverno` describes whether a rule should later be eligible for generated Kyverno rejection:

- `Auto`: default. Future tooling may generate Kyverno if the rule can be evaluated correctly at admission time.
- `Always`: the author expects Kyverno rejection support. Future generation or CI should fail if the rule cannot be translated.
- `Never`: keep the rule as Crossplane render-time validation only, even if it appears translatable.

This field is intentionally named after the developer-facing outcome: invalid resources may be rejected by Kyverno before Crossplane reconciles them.

Kyverno can evaluate more than just the submitted object. It can fetch same-cluster Kubernetes resources through context lookups or CEL resource helpers. Therefore, Kyverno eligibility is not limited to rules over `xr`; it means the same result can be computed correctly during Kubernetes admission.

For v1, the function only parses, defaults, and validates the enum value for `rejectWithKyverno`. It does not generate Kyverno policies, and it does not reject rules because their future Kyverno translation may be impossible. For example, it does not prove whether `rejectWithKyverno: Always` can really be translated. That check belongs to the future Kyverno generator, where CEL AST analysis and Kyverno feature support are available.

Future tooling may add CEL AST analysis to infer dependencies and Kyverno eligibility. Explicit `uses` remains part of v1 so rules are reviewable and later generators do not depend on perfect inference.

## Evaluation Behavior

The function evaluates all applicable rules and reports all validation failures in a stable order.

Rules are evaluated in input order. Validation failure messages are reported in that same order.

Evaluation rules:

- Invalid function input fails before rule evaluation.
- Empty `spec.rules` passes.
- If no rules fail, the function returns no fatal result.
- Required-resource selector errors are fatal developer errors and name the alias that could not be resolved.
- Required resources that are not found are represented as `null` and can be handled by validation rules.
- References to aliases not declared in `uses` are fatal input errors and include the rule id.
- CEL compile errors are fatal developer errors and include the rule id.
- `when` evaluating to false skips the rule.
- `when` evaluation errors are fatal developer errors.
- `assert` evaluating to false creates a validation failure with the rule's `message`.
- `assert` evaluation errors are fatal developer errors and include the rule id.
- Multiple validation failures are returned together in one fatal result.

The implementation must evaluate CEL with bounded cost, using context cancellation and an execution step limit. Pathological rules should fail as developer errors rather than stall reconciliation.

The user-facing error should be concise and action-oriented. Developer diagnostics should include the failed rule ids, skipped rule ids, missing aliases, CEL errors, and the aliases available during evaluation.

## Error UX

Composition authors own the text shown to users through `message`. The function should avoid inventing user-facing explanations from CEL expressions. The `description` field is for developer diagnostics, documentation, and logs; it should not replace the user-facing `message`.

For example, a good message is:

```yaml
message: Referenced Service Bus namespace does not allow this environment.
```

A weaker message would describe the implementation formula instead of the contract:

```yaml
message: required.namespace.spec.environment must equal xr.spec.environment.
```

The latter may still be useful in logs, but it should not be the primary message shown to resource authors.

## Testing Strategy

The first implementation should test the contract rather than any one product team's composition.

Required test areas:

- Input decoding and defaulting, especially `rejectWithKyverno: Auto`.
- Invalid input shapes and invalid enum values.
- Duplicate rule ids.
- Strict `uses` enforcement and undeclared alias references.
- CEL compile and evaluation.
- `when` preconditions.
- Missing fields and `has()` behavior.
- Required resource resolution.
- Required resource not found behavior.
- Required-resource selector stabilization.
- Validation failure aggregation.
- Developer error classification versus user validation failure classification.
- `crossplane render` failure behavior with a human-readable message.

Render tests should include fixtures with same-cluster required resources so local render behavior proves the function works without a live cluster. The test harness should use `crossplane render` with an extra-resources fixture file so the required-resource protocol is exercised in the same way users will debug it locally.

Simple single-object shape checks still belong in XRD schemas and XRD CEL validation. This function should be used for checks that need referenced resources, pipeline state, conditional logic that would be awkward in schema validation, or reusable validation shared across compositions.

## Repository Shape

The repo should begin with a normal Go Crossplane function layout:

```text
crossplane-function-validate/
  apis/
  cmd/
  internal/
    cel/
    input/
    requiredresources/
    validation/
  package/
  tests/
    render/
    fixtures/
  docs/
    superpowers/
      specs/
```

The spec repository starts with this design document only. Implementation scaffolding should happen after the design is reviewed and an implementation plan is written.

## Open Decisions For Implementation Planning

- Exact Crossplane function SDK version.
- Exact formatting of the single fatal result when multiple validation failures are reported together.
- Exact logging and diagnostic-result surface for developer details.
- Exact package layout once the generated Crossplane function scaffold is created.
