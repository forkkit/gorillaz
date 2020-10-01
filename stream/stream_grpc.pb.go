// Code generated by protoc-gen-go-grpc. DO NOT EDIT.

package stream

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion6

// StreamClient is the client API for Stream service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type StreamClient interface {
	Stream(ctx context.Context, in *StreamRequest, opts ...grpc.CallOption) (Stream_StreamClient, error)
	// Gets the initial states and watches for updates
	GetAndWatch(ctx context.Context, in *GetAndWatchRequest, opts ...grpc.CallOption) (Stream_GetAndWatchClient, error)
}

type streamClient struct {
	cc grpc.ClientConnInterface
}

func NewStreamClient(cc grpc.ClientConnInterface) StreamClient {
	return &streamClient{cc}
}

func (c *streamClient) Stream(ctx context.Context, in *StreamRequest, opts ...grpc.CallOption) (Stream_StreamClient, error) {
	stream, err := c.cc.NewStream(ctx, &_Stream_serviceDesc.Streams[0], "/stream.Stream/Stream", opts...)
	if err != nil {
		return nil, err
	}
	x := &streamStreamClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type Stream_StreamClient interface {
	Recv() (*StreamEvent, error)
	grpc.ClientStream
}

type streamStreamClient struct {
	grpc.ClientStream
}

func (x *streamStreamClient) Recv() (*StreamEvent, error) {
	m := new(StreamEvent)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *streamClient) GetAndWatch(ctx context.Context, in *GetAndWatchRequest, opts ...grpc.CallOption) (Stream_GetAndWatchClient, error) {
	stream, err := c.cc.NewStream(ctx, &_Stream_serviceDesc.Streams[1], "/stream.Stream/GetAndWatch", opts...)
	if err != nil {
		return nil, err
	}
	x := &streamGetAndWatchClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type Stream_GetAndWatchClient interface {
	Recv() (*GetAndWatchEvent, error)
	grpc.ClientStream
}

type streamGetAndWatchClient struct {
	grpc.ClientStream
}

func (x *streamGetAndWatchClient) Recv() (*GetAndWatchEvent, error) {
	m := new(GetAndWatchEvent)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// StreamServer is the server API for Stream service.
// All implementations should embed UnimplementedStreamServer
// for forward compatibility
type StreamServer interface {
	Stream(*StreamRequest, Stream_StreamServer) error
	// Gets the initial states and watches for updates
	GetAndWatch(*GetAndWatchRequest, Stream_GetAndWatchServer) error
}

// UnimplementedStreamServer should be embedded to have forward compatible implementations.
type UnimplementedStreamServer struct {
}

func (*UnimplementedStreamServer) Stream(*StreamRequest, Stream_StreamServer) error {
	return status.Errorf(codes.Unimplemented, "method Stream not implemented")
}
func (*UnimplementedStreamServer) GetAndWatch(*GetAndWatchRequest, Stream_GetAndWatchServer) error {
	return status.Errorf(codes.Unimplemented, "method GetAndWatch not implemented")
}

func RegisterStreamServer(s *grpc.Server, srv StreamServer) {
	s.RegisterService(&_Stream_serviceDesc, srv)
}

func _Stream_Stream_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(StreamRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(StreamServer).Stream(m, &streamStreamServer{stream})
}

type Stream_StreamServer interface {
	Send(*StreamEvent) error
	grpc.ServerStream
}

type streamStreamServer struct {
	grpc.ServerStream
}

func (x *streamStreamServer) Send(m *StreamEvent) error {
	return x.ServerStream.SendMsg(m)
}

func _Stream_GetAndWatch_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(GetAndWatchRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(StreamServer).GetAndWatch(m, &streamGetAndWatchServer{stream})
}

type Stream_GetAndWatchServer interface {
	Send(*GetAndWatchEvent) error
	grpc.ServerStream
}

type streamGetAndWatchServer struct {
	grpc.ServerStream
}

func (x *streamGetAndWatchServer) Send(m *GetAndWatchEvent) error {
	return x.ServerStream.SendMsg(m)
}

var _Stream_serviceDesc = grpc.ServiceDesc{
	ServiceName: "stream.Stream",
	HandlerType: (*StreamServer)(nil),
	Methods:     []grpc.MethodDesc{},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "Stream",
			Handler:       _Stream_Stream_Handler,
			ServerStreams: true,
		},
		{
			StreamName:    "GetAndWatch",
			Handler:       _Stream_GetAndWatch_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "stream.proto",
}