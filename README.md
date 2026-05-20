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
It also does not expose claim resources as CEL aliases; Crossplane function requests do not provide the claim object directly.
