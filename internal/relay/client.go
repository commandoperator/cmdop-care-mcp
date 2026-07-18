// Package relay is the thin gRPC transport this artifact needs: dial the
// user's own cmdop relay, attach the Bearer token read from the OS keyring
// (see internal/credentials), and call exactly three RPCs — GetMachineCare,
// RunMachineCareDiagnostic, and ListSessions. No other RPC is wired, and the
// generated stub package (internal/carepb) does not even define any mutation
// RPC, so there is no code path that could reach one.
package relay

import (
	"context"
	"fmt"
	"net"
	"time"

	"google.golang.org/grpc"
	grpccredentials "google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/commandoperator/cmdop-care-mcp/internal/carepb"
	"github.com/commandoperator/cmdop-care-mcp/internal/credentials"
)

// DefaultAddr is the loopback address the embedded cmdop relay binds its
// gRPC π-pair to by default (see the private repo's
// internal/foundation/netports.RelayGRPCPort, currently 63142). This
// artifact does not import that package (it is product-internal); the
// constant is duplicated here deliberately, at a fixed, documented value.
const DefaultAddr = "127.0.0.1:63142"

// Client wraps one gRPC connection to the user's own cmdop relay plus the
// two generated service stubs this artifact calls.
type Client struct {
	conn  *grpc.ClientConn
	care  carepb.AiAgentServiceClient
	sess  carepb.SessionServiceClient
	token string
}

// isLoopback reports whether addr's host is a loopback address. Mirrors the
// private repo's client.isLoopbackServer: on loopback the token never
// leaves the machine, so a plaintext (non-TLS) dial is acceptable; anywhere
// else TLS is mandatory before a Bearer token is ever sent.
func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Dial connects to the relay at addr using the given Bearer token. When addr
// is empty, DefaultAddr (the local embedded relay) is used.
func Dial(addr, token string) (*Client, error) {
	if addr == "" {
		addr = DefaultAddr
	}
	if token == "" {
		return nil, fmt.Errorf("relay: empty token")
	}

	var transport grpc.DialOption
	if isLoopback(addr) {
		transport = grpc.WithTransportCredentials(insecure.NewCredentials())
	} else {
		transport = grpc.WithTransportCredentials(grpccredentials.NewTLS(nil))
	}

	dialCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(dialCtx, addr, transport, grpc.WithBlock()) //nolint:staticcheck // grpc.DialContext is deprecated upstream but still the correct blocking-dial API for this short-lived stdio process
	if err != nil {
		return nil, fmt.Errorf("relay: dial %s: %w", addr, err)
	}

	return &Client{
		conn:  conn,
		care:  carepb.NewAiAgentServiceClient(conn),
		sess:  carepb.NewSessionServiceClient(conn),
		token: token,
	}, nil
}

// NewFromConn builds a Client over an already-established *grpc.ClientConn
// and a token. Exported (not test-only, since Go's internal-package
// visibility already scopes it to this module) so tests can point the
// client at an in-memory bufconn fake server instead of a real network dial.
func NewFromConn(conn *grpc.ClientConn, token string) *Client {
	return &Client{
		conn:  conn,
		care:  carepb.NewAiAgentServiceClient(conn),
		sess:  carepb.NewSessionServiceClient(conn),
		token: token,
	}
}

// Close releases the underlying gRPC connection.
func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// authCtx attaches the Bearer token as gRPC metadata, matching the wire
// contract every cmdop relay expects on every authenticated RPC.
func (c *Client) authCtx(ctx context.Context) context.Context {
	md := metadata.Pairs("authorization", "Bearer "+c.token)
	return metadata.NewOutgoingContext(ctx, md)
}

// Machine is the public, redacted machine roster projection. It deliberately
// has NO field carrying the relay's internal machine UUID — see the
// project's security review §5 ("Decision for the public artifact: keep the
// raw UUID out of the rendered and structured output entirely").
type Machine struct {
	Host   string `json:"host"`
	Online bool   `json:"online"`
	OS     string `json:"os"`
}

// ListMachines fetches the fleet roster and projects it onto the redacted
// Machine shape — no machine ID/UUID is retained past this call.
func (c *Client) ListMachines(ctx context.Context) ([]Machine, error) {
	resp, err := c.sess.ListSessions(c.authCtx(ctx), &carepb.ListSessionsRequest{})
	if err != nil {
		return nil, fmt.Errorf("relay: list machines: %w", err)
	}
	out := make([]Machine, 0, len(resp.GetSessions()))
	for _, s := range resp.GetSessions() {
		out = append(out, Machine{
			Host:   s.GetDisplayName(),
			Online: s.GetStatus() == carepb.SessionStatus_SESSION_STATUS_ONLINE,
			OS:     s.GetOs(),
		})
	}
	return out, nil
}

// CareStatus fetches the durable Care projection for one machine.
func (c *Client) CareStatus(ctx context.Context, machine string) (CareStatus, error) {
	resp, err := c.care.GetMachineCare(c.authCtx(ctx), &carepb.GetMachineCareRequest{Machine: machine})
	if err != nil {
		return CareStatus{}, fmt.Errorf("relay: care status for %q: %w", machine, err)
	}
	if resp == nil || resp.GetMachineId() == "" || (resp.GetReported() && resp.GetSnapshot() == nil) {
		return CareStatus{}, fmt.Errorf("relay: invalid care status response for %q", machine)
	}
	if !resp.GetReported() {
		return CareStatus{Findings: []CareFinding{}}, nil
	}
	return projectCareStatus(resp.GetSnapshot()), nil
}

// CareStorageInventory fetches the latest storage inventory for one machine.
func (c *Client) CareStorageInventory(ctx context.Context, machine string) (CareStorageInventory, error) {
	resp, err := c.care.GetMachineCare(c.authCtx(ctx), &carepb.GetMachineCareRequest{Machine: machine})
	if err != nil {
		return CareStorageInventory{}, fmt.Errorf("relay: care storage inventory for %q: %w", machine, err)
	}
	if resp == nil || resp.GetMachineId() == "" {
		return CareStorageInventory{}, fmt.Errorf("relay: invalid care storage response for %q", machine)
	}
	inv := resp.GetSnapshot().GetLatestStorageInventory()
	if !resp.GetReported() || inv == nil {
		return CareStorageInventory{}, fmt.Errorf("relay: no reported storage inventory for %q", machine)
	}
	return projectCareStorage(inv), nil
}

// CareDiagnose resolves the machine's stable ID via GetMachineCare, forwards
// any keyring-cached PIN for that ID automatically (never accepted as a
// tool argument — see the care_diagnose tool description), and runs the
// bounded live diagnostic.
func (c *Client) CareDiagnose(ctx context.Context, machine string) (CareDiagnostic, error) {
	projection, err := c.care.GetMachineCare(c.authCtx(ctx), &carepb.GetMachineCareRequest{Machine: machine})
	if err != nil {
		return CareDiagnostic{}, fmt.Errorf("relay: resolve %q for diagnostic: %w", machine, err)
	}
	if projection == nil || projection.GetMachineId() == "" {
		return CareDiagnostic{}, fmt.Errorf("relay: could not resolve machine %q", machine)
	}

	pin, pinErr := credentials.MachinePIN(projection.GetMachineId())
	if pinErr != nil && pinErr != credentials.ErrNoPINCached {
		// A keyring read error (not "simply absent") is worth surfacing, but
		// fails closed by proceeding with an empty PIN — matching the relay's
		// own contract: an unarmed target ignores it, an armed target denies it.
		pin = ""
	}

	resp, err := c.care.RunMachineCareDiagnostic(c.authCtx(ctx), &carepb.RunMachineCareDiagnosticRequest{
		Machine:    projection.GetMachineId(),
		MachinePin: pin,
	})
	if err != nil {
		return CareDiagnostic{}, fmt.Errorf("relay: diagnostic for %q: %w", machine, err)
	}
	if resp == nil || resp.GetMachineId() != projection.GetMachineId() || resp.GetDiagnostic() == nil {
		return CareDiagnostic{}, fmt.Errorf("relay: invalid diagnostic response for %q", machine)
	}
	return projectCareDiagnostic(resp.GetDiagnostic()), nil
}
