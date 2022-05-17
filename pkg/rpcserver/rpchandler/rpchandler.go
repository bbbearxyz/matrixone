// Code generated by Kitex v0.2.1. DO NOT EDIT.

package rpchandler

import (
	"context"
	"fmt"
	"github.com/cloudwego/kitex/client"
	kitex "github.com/cloudwego/kitex/pkg/serviceinfo"
	"github.com/cloudwego/kitex/pkg/streaming"
	"github.com/matrixorigin/matrixone/pkg/rpcserver/message"
	"google.golang.org/protobuf/proto"
)

func serviceInfo() *kitex.ServiceInfo {
	return rPCHandlerServiceInfo
}

var rPCHandlerServiceInfo = NewServiceInfo()

func NewServiceInfo() *kitex.ServiceInfo {
	serviceName := "RPCHandler"
	handlerType := (*message.RPCHandler)(nil)
	methods := map[string]kitex.MethodInfo{
		"Process": kitex.NewMethodInfo(processHandler, newProcessArgs, newProcessResult, false),
	}
	extra := map[string]interface{}{
		"PackageName": "message",
	}
	extra["streaming"] = true
	svcInfo := &kitex.ServiceInfo{
		ServiceName:     serviceName,
		HandlerType:     handlerType,
		Methods:         methods,
		PayloadCodec:    kitex.Protobuf,
		KiteXGenVersion: "v0.2.1",
		Extra:           extra,
	}
	return svcInfo
}

func processHandler(ctx context.Context, handler interface{}, arg, result interface{}) error {
	st := arg.(*streaming.Args).Stream
	stream := &rPCHandlerProcessServer{st}
	req := new(message.Message)
	if err := st.RecvMsg(req); err != nil {
		return err
	}
	return handler.(message.RPCHandler).Process(req, stream)
}

type rPCHandlerProcessClient struct {
	streaming.Stream
}

func (x *rPCHandlerProcessClient) Recv() (*message.Message, error) {
	m := new(message.Message)
	return m, x.Stream.RecvMsg(m)
}

type rPCHandlerProcessServer struct {
	streaming.Stream
}

func (x *rPCHandlerProcessServer) Send(m *message.Message) error {
	return x.Stream.SendMsg(m)
}

func newProcessArgs() interface{} {
	return &ProcessArgs{}
}

func newProcessResult() interface{} {
	return &ProcessResult{}
}

type ProcessArgs struct {
	Req *message.Message
}

func (p *ProcessArgs) Marshal(out []byte) ([]byte, error) {
	if !p.IsSetReq() {
		return out, fmt.Errorf("No req in ProcessArgs")
	}
	return proto.Marshal(p.Req)
}

func (p *ProcessArgs) Unmarshal(in []byte) error {
	msg := new(message.Message)
	if err := proto.Unmarshal(in, msg); err != nil {
		return err
	}
	p.Req = msg
	return nil
}

var ProcessArgs_Req_DEFAULT *message.Message

func (p *ProcessArgs) GetReq() *message.Message {
	if !p.IsSetReq() {
		return ProcessArgs_Req_DEFAULT
	}
	return p.Req
}

func (p *ProcessArgs) IsSetReq() bool {
	return p.Req != nil
}

type ProcessResult struct {
	Success *message.Message
}

var ProcessResult_Success_DEFAULT *message.Message

func (p *ProcessResult) Marshal(out []byte) ([]byte, error) {
	if !p.IsSetSuccess() {
		return out, fmt.Errorf("No req in ProcessResult")
	}
	return proto.Marshal(p.Success)
}

func (p *ProcessResult) Unmarshal(in []byte) error {
	msg := new(message.Message)
	if err := proto.Unmarshal(in, msg); err != nil {
		return err
	}
	p.Success = msg
	return nil
}

func (p *ProcessResult) GetSuccess() *message.Message {
	if !p.IsSetSuccess() {
		return ProcessResult_Success_DEFAULT
	}
	return p.Success
}

func (p *ProcessResult) SetSuccess(x interface{}) {
	p.Success = x.(*message.Message)
}

func (p *ProcessResult) IsSetSuccess() bool {
	return p.Success != nil
}

type kClient struct {
	c client.Client
}

func newServiceClient(c client.Client) *kClient {
	return &kClient{
		c: c,
	}
}

func (p *kClient) Process(ctx context.Context, req *message.Message) (RPCHandler_ProcessClient, error) {
	streamClient, ok := p.c.(client.Streaming)
	if !ok {
		return nil, fmt.Errorf("client not support streaming")
	}
	res := new(streaming.Result)
	err := streamClient.Stream(ctx, "Process", nil, res)
	if err != nil {
		return nil, err
	}
	stream := &rPCHandlerProcessClient{res.Stream}
	if err := stream.Stream.SendMsg(req); err != nil {
		return nil, err
	}
	if err := stream.Stream.Close(); err != nil {
		return nil, err
	}
	return stream, nil
}
