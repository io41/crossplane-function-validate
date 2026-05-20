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
