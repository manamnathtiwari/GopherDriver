// Package proto defines the gRPC service interface for GopherDrive.
//
// In a full protoc workflow you would generate this with protoc-gen-go-grpc.
// This hand-written version keeps the project self-contained.
package proto

import (
	"context"

	"google.golang.org/grpc"
)

// GopherDriveServer is the server-side interface for the MetadataService.
type GopherDriveServer interface {
	RegisterFile(context.Context, *RegisterFileRequest) (*RegisterFileResponse, error)
	UpdateStatus(context.Context, *UpdateStatusRequest) (*UpdateStatusResponse, error)
}

// GopherDriveClient is the client-side interface for the MetadataService.
type GopherDriveClient interface {
	RegisterFile(ctx context.Context, in *RegisterFileRequest, opts ...grpc.CallOption) (*RegisterFileResponse, error)
	UpdateStatus(ctx context.Context, in *UpdateStatusRequest, opts ...grpc.CallOption) (*UpdateStatusResponse, error)
}

// ---- server registration ----

// ServiceDesc is the grpc.ServiceDesc for the MetadataService.
var ServiceDesc = grpc.ServiceDesc{
	ServiceName: "gopherdrive.MetadataService",
	HandlerType: (*GopherDriveServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "RegisterFile",
			Handler:    _GopherDrive_RegisterFile_Handler,
		},
		{
			MethodName: "UpdateStatus",
			Handler:    _GopherDrive_UpdateStatus_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "proto/gopherdrive.proto",
}

// RegisterGopherDriveServer registers the server implementation with a gRPC server.
func RegisterGopherDriveServer(s *grpc.Server, srv GopherDriveServer) {
	s.RegisterService(&ServiceDesc, srv)
}

func _GopherDrive_RegisterFile_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, _ grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RegisterFileRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	return srv.(GopherDriveServer).RegisterFile(ctx, in)
}

func _GopherDrive_UpdateStatus_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, _ grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(UpdateStatusRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	return srv.(GopherDriveServer).UpdateStatus(ctx, in)
}

// ---- client implementation ----

type gopherDriveClient struct {
	cc grpc.ClientConnInterface
}

// NewGopherDriveClient creates a new MetadataService gRPC client.
func NewGopherDriveClient(cc grpc.ClientConnInterface) GopherDriveClient {
	return &gopherDriveClient{cc: cc}
}

func (c *gopherDriveClient) RegisterFile(ctx context.Context, in *RegisterFileRequest, opts ...grpc.CallOption) (*RegisterFileResponse, error) {
	out := new(RegisterFileResponse)
	err := c.cc.Invoke(ctx, "/gopherdrive.MetadataService/RegisterFile", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *gopherDriveClient) UpdateStatus(ctx context.Context, in *UpdateStatusRequest, opts ...grpc.CallOption) (*UpdateStatusResponse, error) {
	out := new(UpdateStatusResponse)
	err := c.cc.Invoke(ctx, "/gopherdrive.MetadataService/UpdateStatus", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}
