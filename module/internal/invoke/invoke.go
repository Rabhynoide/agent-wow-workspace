// Package invoke calls RPC methods of declared module dependencies.
package invoke

import (
	"context"

	modv1 "github.com/agent-wow/agent-wow/pkg/modules/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// Call invokes module.method (the rpc alias from module.yaml) and decodes the result into resp.
func Call(ctx context.Context, s modv1.SessionClient, module, method string, req, resp proto.Message) error {
	a, err := anypb.New(req)
	if err != nil {
		return err
	}
	r, err := s.InvokeModule(ctx, &modv1.InvokeModuleRequest{Module: module, Method: method, Request: a})
	if err != nil {
		return err
	}
	return r.Result.UnmarshalTo(resp)
}
