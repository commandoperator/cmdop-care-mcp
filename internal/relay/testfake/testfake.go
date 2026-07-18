// Package testfake provides an in-memory (bufconn) fake relay server
// implementing carepb.AiAgentServiceServer and carepb.SessionServiceServer,
// so relay/tools tests exercise real gRPC wire behavior without a real
// network, a real cmdop relay, or a real OS keyring.
package testfake

import (
	"context"
	"errors"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/commandoperator/cmdop-care-mcp/internal/carepb"
)

// Server is a fake cmdop relay: it answers GetMachineCare,
// RunMachineCareDiagnostic, and ListSessions from in-memory fixtures the
// test configures, and can simulate a hang (for timeout tests) or an
// authentication check (rejecting a wrong Bearer token).
type Server struct {
	carepb.UnimplementedAiAgentServiceServer
	carepb.UnimplementedSessionServiceServer

	Sessions []*carepb.SessionInfo
	// Care maps a "machine" lookup key (display name OR resolved ID — the
	// fake does no fuzzy resolution) to a canned response.
	Care map[string]*carepb.GetMachineCareResponse
	// Diagnostics maps a resolved machine ID to a canned diagnostic response.
	Diagnostics map[string]*carepb.RunMachineCareDiagnosticResponse

	// Hang, when true, makes every RPC block until ctx is done — used to
	// prove the client-side per-call timeout actually cuts off a stuck
	// backend instead of hanging forever.
	Hang bool

	// CallCount lets a test assert how many times an RPC was actually
	// invoked (e.g. to prove a rate limiter blocked calls before they ever
	// reached the network).
	CallCount int

	lis *bufconn.Listener
	srv *grpc.Server
}

// Start boots the fake server over an in-memory bufconn listener and
// returns a dial function compatible with grpc.DialContext's
// WithContextDialer.
func Start(t interface {
	Cleanup(func())
	Fatalf(string, ...any)
}, s *Server) func(context.Context, string) (net.Conn, error) {
	s.lis = bufconn.Listen(1024 * 1024)
	s.srv = grpc.NewServer()
	carepb.RegisterAiAgentServiceServer(s.srv, s)
	carepb.RegisterSessionServiceServer(s.srv, s)
	go func() {
		_ = s.srv.Serve(s.lis)
	}()
	t.Cleanup(func() {
		s.srv.Stop()
	})
	return func(ctx context.Context, _ string) (net.Conn, error) {
		return s.lis.DialContext(ctx)
	}
}

// DialOptions returns the grpc.DialOption set a test client should use
// against this fake (insecure transport + the bufconn dialer).
func DialOptions(dialer func(context.Context, string) (net.Conn, error)) []grpc.DialOption {
	return []grpc.DialOption{
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
}

// Dial is a one-call convenience wrapping Start + grpc.NewClient for tests
// that just want a ready *grpc.ClientConn against a fake Server.
func Dial(t interface {
	Cleanup(func())
	Fatalf(string, ...any)
}, s *Server) *grpc.ClientConn {
	dialer := Start(t, s)
	conn, err := grpc.NewClient("passthrough:///bufnet", DialOptions(dialer)...)
	if err != nil {
		t.Fatalf("dial fake: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func (s *Server) GetMachineCare(ctx context.Context, req *carepb.GetMachineCareRequest) (*carepb.GetMachineCareResponse, error) {
	s.CallCount++
	if s.Hang {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if resp, ok := s.Care[req.GetMachine()]; ok {
		return resp, nil
	}
	return nil, errors.New("testfake: unknown machine")
}

func (s *Server) RunMachineCareDiagnostic(ctx context.Context, req *carepb.RunMachineCareDiagnosticRequest) (*carepb.RunMachineCareDiagnosticResponse, error) {
	s.CallCount++
	if s.Hang {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if resp, ok := s.Diagnostics[req.GetMachine()]; ok {
		return resp, nil
	}
	return nil, errors.New("testfake: no diagnostic for machine")
}

func (s *Server) ListSessions(ctx context.Context, _ *carepb.ListSessionsRequest) (*carepb.ListSessionsResponse, error) {
	s.CallCount++
	if s.Hang {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return &carepb.ListSessionsResponse{Sessions: s.Sessions}, nil
}
