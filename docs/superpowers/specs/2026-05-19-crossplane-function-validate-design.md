# crossplane-function-validate Design

## Summary

`crossplane-function-validate` is a Go-based Crossplane composition function for render-time and reconcile-time validation. It lets composition authors move validation out of ad hoc template logic and into declarative CEL rules with clear, user-owned messages.

The first version validates only data available through the Crossplane function pipeline: the composite resource, claim, pipeline context, observed and desired resources, and named required Kubernetes resources. It does not call arbitrary HTTP services, cloud APIs, or remote clusters.

The design also leaves room for a future Kyverno generator. Rules can opt into, out of, or defer generated Kyverno rejection with `rejectWithKyverno`, so the same validation source can eventually support both Crossplane render failures and Kubernetes admission rejection where technically possible.

## Goals

- Provide a reusable Crossplane function for validation-only pipeline steps.
- Use CEL as the rule language.
- Expose a stable, purpose-built CEL data model instead of the raw Crossplane SDK request.
- Support named required Kubernetes resources so rules can validate referenced objects.
- Produce human-readable validation messages during `crossplane render` and reconciliation.
- Evaluate all applicable rules and report all validation failures in a stable order.
- Keep v1 narrow enough to be simple to test and operate.
- Preserve a clean future path for deriving Kyverno policies from eligible rules.

## Non-Goals

- Generate Kyverno policies in v1.
- Query arbitrary HTTP endpoints, cloud provider APIs, or remote clusters in v1.
- Mutate desired resources.
- Replace XRD schema validation for simple static checks.
- Expose the raw Crossplane `RunFunctionRequest` as the public CEL API.
- Become a general policy engine.

## Architecture

The function runs as a normal Crossplane composition function step. It should usually run early in the pipeline, before later steps assume referenced resources are compatible.

The function accepts a `Rules` input document, resolves the declared inputs, compiles and evaluates CEL rules, and returns one fatal Crossplane function result when validation fails. That result contains all failed rule messages in a stable order. The function does not create, update, or delete desired resources.

The CEL environment exposes stable aliases:

- `xr`: the composite resource.
- `claim`: the claim resource, when present.
- `context`: Crossplane pipeline context.
- `observed`: observed composed resources.
- `desired`: desired composed resources.
- `required`: named required Kubernetes resources.

Named required resources must be resolved through Crossplane's required resource mechanism, not a direct Kubernetes client. That keeps local `crossplane render` behavior aligned with in-cluster reconciliation.

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
    - id: namespace-environment-matches-topic
      description: Referenced Service Bus namespace must allow this topic environment.
      uses:
        - xr
        - required.namespace
      rejectWithKyverno: Auto
      when: has(required.namespace.spec.environment)
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

`nameFrom` and `namespaceFrom` are evaluated against admission-visible aliases such as `xr` and `claim` where possible, but v1 should keep implementation support focused on the Crossplane function runtime.

### Rules

Each rule supports:

- `id`: stable identifier used in logs, test assertions, and diagnostics.
- `description`: developer-facing explanation of the rule.
- `uses`: explicit aliases the rule depends on.
- `rejectWithKyverno`: future Kyverno generation preference. Defaults to `Auto`.
- `when`: optional CEL precondition. If false, the rule is skipped.
- `assert`: CEL expression that must evaluate to true.
- `message`: user-facing validation failure message.

Rule messages should tell users what is wrong or what action to take. They should not expose implementation formulas unless that formula is the actual user contract.

## Kyverno Future Path

`rejectWithKyverno` describes whether a rule should later be eligible for generated Kyverno rejection:

- `Auto`: default. Future tooling may generate Kyverno if the rule can be evaluated correctly at admission time.
- `Always`: the author expects Kyverno rejection support. Future generation or CI should fail if the rule cannot be translated.
- `Never`: keep the rule as Crossplane render-time validation only, even if it appears translatable.

This field is intentionally named after the developer-facing outcome: invalid resources may be rejected by Kyverno before Crossplane reconciles them.

Kyverno can evaluate more than just the submitted object. It can fetch same-cluster Kubernetes resources through context lookups or CEL resource helpers. Therefore, Kyverno eligibility is not limited to rules over `xr` or `claim`; it means the same result can be computed correctly during Kubernetes admission.

For v1, the function only parses and validates `rejectWithKyverno`. It does not generate Kyverno policies. It may reject obvious contradictions, such as `rejectWithKyverno: Always` on a rule that depends on Crossplane-only data like `desired`, `observed`, or pipeline-produced context.

Future tooling may add CEL AST analysis to infer dependencies and Kyverno eligibility. Explicit `uses` remains part of v1 so rules are reviewable and later generators do not depend on perfect inference.

## Evaluation Behavior

The function evaluates all applicable rules and reports all validation failures in a stable order.

Evaluation rules:

- Invalid function input fails before rule evaluation.
- Required resource lookup failures are fatal and name the alias that could not be resolved.
- CEL compile errors are fatal developer errors and include the rule id.
- `when` evaluating to false skips the rule.
- `when` evaluation errors are fatal developer errors.
- `assert` evaluating to false creates a validation failure with the rule's `message`.
- `assert` evaluation errors are fatal developer errors and include the rule id.
- Multiple validation failures are returned together.

The user-facing error should be concise and action-oriented. Developer diagnostics should include the failed rule ids, skipped rule ids, missing aliases, CEL errors, and the aliases available during evaluation.

## Error UX

Composition authors own the text shown to users through `message`. The function should avoid inventing user-facing explanations from CEL expressions.

For example, a good message is:

```yaml
message: Referenced Service Bus namespace does not allow this environment.
```

A weaker message would describe the implementation formula instead of the contract:

```yaml
message: required.namespace.spec.environment must equal xr.spec.environment.
```

The latter may still be useful in logs, but it should not be the primary message shown to claim authors.

## Testing Strategy

The first implementation should test the contract rather than any one product team's composition.

Required test areas:

- Input decoding and defaulting, especially `rejectWithKyverno: Auto`.
- Invalid input shapes and invalid enum values.
- CEL compile and evaluation.
- `when` preconditions.
- Missing fields and `has()` behavior.
- Required resource resolution.
- Validation failure aggregation.
- Developer error classification versus user validation failure classification.
- `crossplane render` failure behavior with a human-readable message.

Render tests should include fixtures with same-cluster required resources so local render behavior proves the function works without a live cluster.

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
- Exact CEL library and Kubernetes object typing strategy.
- Exact formatting of the single fatal result when multiple validation failures are reported together.
- Exact package layout once the generated Crossplane function scaffold is created.
- How strict v1 should be when `uses` does not match aliases found in CEL expressions.
