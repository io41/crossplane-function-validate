package main

import (
	"context"
	"errors"
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
		response.Fatal(rsp, errors.New(formatFailures(out.Failures)))
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
