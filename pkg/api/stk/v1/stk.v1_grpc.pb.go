// Code generated by protoc-gen-go-grpc. DO NOT EDIT.

package stk_v1

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion7

// StkPushV1Client is the client API for StkPushV1 service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type StkPushV1Client interface {
	// Initiates mpesa stk.
	InitiateSTK(ctx context.Context, in *InitiateSTKRequest, opts ...grpc.CallOption) (*InitiateSTKResponse, error)
	// Retrieves a single stk transaction.
	GetStkTransaction(ctx context.Context, in *GetStkTransactionRequest, opts ...grpc.CallOption) (*StkTransaction, error)
	// Retrieves a collection of stk transactions.
	ListStkTransactions(ctx context.Context, in *ListStkTransactionsRequest, opts ...grpc.CallOption) (*ListStkTransactionsResponse, error)
	// Processes stk transaction updating its status.
	ProcessStkTransaction(ctx context.Context, in *ProcessStkTransactionRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	// Publishes stk transaction to consumers.
	PublishStkTransaction(ctx context.Context, in *PublishStkTransactionRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
}

type stkPushV1Client struct {
	cc grpc.ClientConnInterface
}

func NewStkPushV1Client(cc grpc.ClientConnInterface) StkPushV1Client {
	return &stkPushV1Client{cc}
}

func (c *stkPushV1Client) InitiateSTK(ctx context.Context, in *InitiateSTKRequest, opts ...grpc.CallOption) (*InitiateSTKResponse, error) {
	out := new(InitiateSTKResponse)
	err := c.cc.Invoke(ctx, "/gidyon.mpesastk.StkPushV1/InitiateSTK", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *stkPushV1Client) GetStkTransaction(ctx context.Context, in *GetStkTransactionRequest, opts ...grpc.CallOption) (*StkTransaction, error) {
	out := new(StkTransaction)
	err := c.cc.Invoke(ctx, "/gidyon.mpesastk.StkPushV1/GetStkTransaction", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *stkPushV1Client) ListStkTransactions(ctx context.Context, in *ListStkTransactionsRequest, opts ...grpc.CallOption) (*ListStkTransactionsResponse, error) {
	out := new(ListStkTransactionsResponse)
	err := c.cc.Invoke(ctx, "/gidyon.mpesastk.StkPushV1/ListStkTransactions", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *stkPushV1Client) ProcessStkTransaction(ctx context.Context, in *ProcessStkTransactionRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out := new(emptypb.Empty)
	err := c.cc.Invoke(ctx, "/gidyon.mpesastk.StkPushV1/ProcessStkTransaction", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *stkPushV1Client) PublishStkTransaction(ctx context.Context, in *PublishStkTransactionRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out := new(emptypb.Empty)
	err := c.cc.Invoke(ctx, "/gidyon.mpesastk.StkPushV1/PublishStkTransaction", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// StkPushV1Server is the server API for StkPushV1 service.
// All implementations must embed UnimplementedStkPushV1Server
// for forward compatibility
type StkPushV1Server interface {
	// Initiates mpesa stk.
	InitiateSTK(context.Context, *InitiateSTKRequest) (*InitiateSTKResponse, error)
	// Retrieves a single stk transaction.
	GetStkTransaction(context.Context, *GetStkTransactionRequest) (*StkTransaction, error)
	// Retrieves a collection of stk transactions.
	ListStkTransactions(context.Context, *ListStkTransactionsRequest) (*ListStkTransactionsResponse, error)
	// Processes stk transaction updating its status.
	ProcessStkTransaction(context.Context, *ProcessStkTransactionRequest) (*emptypb.Empty, error)
	// Publishes stk transaction to consumers.
	PublishStkTransaction(context.Context, *PublishStkTransactionRequest) (*emptypb.Empty, error)
	mustEmbedUnimplementedStkPushV1Server()
}

// UnimplementedStkPushV1Server must be embedded to have forward compatible implementations.
type UnimplementedStkPushV1Server struct {
}

func (UnimplementedStkPushV1Server) InitiateSTK(context.Context, *InitiateSTKRequest) (*InitiateSTKResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method InitiateSTK not implemented")
}
func (UnimplementedStkPushV1Server) GetStkTransaction(context.Context, *GetStkTransactionRequest) (*StkTransaction, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetStkTransaction not implemented")
}
func (UnimplementedStkPushV1Server) ListStkTransactions(context.Context, *ListStkTransactionsRequest) (*ListStkTransactionsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListStkTransactions not implemented")
}
func (UnimplementedStkPushV1Server) ProcessStkTransaction(context.Context, *ProcessStkTransactionRequest) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ProcessStkTransaction not implemented")
}
func (UnimplementedStkPushV1Server) PublishStkTransaction(context.Context, *PublishStkTransactionRequest) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method PublishStkTransaction not implemented")
}
func (UnimplementedStkPushV1Server) mustEmbedUnimplementedStkPushV1Server() {}

// UnsafeStkPushV1Server may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to StkPushV1Server will
// result in compilation errors.
type UnsafeStkPushV1Server interface {
	mustEmbedUnimplementedStkPushV1Server()
}

func RegisterStkPushV1Server(s grpc.ServiceRegistrar, srv StkPushV1Server) {
	s.RegisterService(&_StkPushV1_serviceDesc, srv)
}

func _StkPushV1_InitiateSTK_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(InitiateSTKRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(StkPushV1Server).InitiateSTK(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/gidyon.mpesastk.StkPushV1/InitiateSTK",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(StkPushV1Server).InitiateSTK(ctx, req.(*InitiateSTKRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _StkPushV1_GetStkTransaction_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetStkTransactionRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(StkPushV1Server).GetStkTransaction(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/gidyon.mpesastk.StkPushV1/GetStkTransaction",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(StkPushV1Server).GetStkTransaction(ctx, req.(*GetStkTransactionRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _StkPushV1_ListStkTransactions_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListStkTransactionsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(StkPushV1Server).ListStkTransactions(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/gidyon.mpesastk.StkPushV1/ListStkTransactions",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(StkPushV1Server).ListStkTransactions(ctx, req.(*ListStkTransactionsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _StkPushV1_ProcessStkTransaction_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ProcessStkTransactionRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(StkPushV1Server).ProcessStkTransaction(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/gidyon.mpesastk.StkPushV1/ProcessStkTransaction",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(StkPushV1Server).ProcessStkTransaction(ctx, req.(*ProcessStkTransactionRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _StkPushV1_PublishStkTransaction_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(PublishStkTransactionRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(StkPushV1Server).PublishStkTransaction(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/gidyon.mpesastk.StkPushV1/PublishStkTransaction",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(StkPushV1Server).PublishStkTransaction(ctx, req.(*PublishStkTransactionRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _StkPushV1_serviceDesc = grpc.ServiceDesc{
	ServiceName: "gidyon.mpesastk.StkPushV1",
	HandlerType: (*StkPushV1Server)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "InitiateSTK",
			Handler:    _StkPushV1_InitiateSTK_Handler,
		},
		{
			MethodName: "GetStkTransaction",
			Handler:    _StkPushV1_GetStkTransaction_Handler,
		},
		{
			MethodName: "ListStkTransactions",
			Handler:    _StkPushV1_ListStkTransactions_Handler,
		},
		{
			MethodName: "ProcessStkTransaction",
			Handler:    _StkPushV1_ProcessStkTransaction_Handler,
		},
		{
			MethodName: "PublishStkTransaction",
			Handler:    _StkPushV1_PublishStkTransaction_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "stk.v1.proto",
}
