package resources

import (
	"context"
	"fmt"
	"io"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/debugcontainer"
	"github.com/sophotechlabs/spinoza/internal/exec"
	"github.com/sophotechlabs/spinoza/internal/portforward"
)

func (m *Manager) StartForward(ctx context.Context, target portforward.Target, port int32) (api.PortForward, error) {
	if m.forwards == nil {
		return api.PortForward{}, fmt.Errorf("%w: port forwarding is not wired up", api.ErrInternal)
	}
	return m.forwards.Start(ctx, target, port)
}

func (m *Manager) Forwards() []api.PortForward {
	if m.forwards == nil {
		return []api.PortForward{}
	}
	return m.forwards.List()
}

func (m *Manager) StopForward(id string) error {
	if m.forwards == nil {
		return fmt.Errorf("%w: port forwarding is not wired up", api.ErrInternal)
	}
	return m.forwards.Stop(id)
}

func (m *Manager) ExecSupport(ctx context.Context, req exec.Request) (api.ExecSupport, error) {
	if m.shells == nil {
		return api.ExecSupport{}, fmt.Errorf("%w: exec is not wired up", api.ErrInternal)
	}
	return m.shells.Support(ctx, req)
}

func (m *Manager) StartExec(ctx context.Context, req exec.Request, stdout io.Writer) (*exec.Session, error) {
	if m.shells == nil {
		return nil, fmt.Errorf("%w: exec is not wired up", api.ErrInternal)
	}
	return m.shells.Start(ctx, req, stdout)
}

func (m *Manager) DebugSupport(ctx context.Context, namespace, pod string) api.DebugSupport {
	if m.debugger == nil {
		return api.DebugSupport{Namespace: namespace, Pod: pod, Allowed: false, Reason: debugcontainer.ErrUnavailable.Error()}
	}
	return m.debugger.Allowed(ctx, namespace, pod)
}

func (m *Manager) StartDebug(ctx context.Context, req debugcontainer.Request) (api.DebugSession, error) {
	if m.debugger == nil {
		return api.DebugSession{}, debugcontainer.ErrUnavailable
	}
	return m.debugger.Ensure(ctx, req)
}

func (m *Manager) NodeShellSupport(ctx context.Context, node string) api.NodeShellSupport {
	if m.nodeShells == nil {
		return api.NodeShellSupport{Node: node, Reason: "node shells are not wired up"}
	}
	return m.nodeShells.Support(ctx, node)
}

func (m *Manager) StartNodeShell(ctx context.Context, node string) (api.NodeShellSession, error) {
	if m.nodeShells == nil {
		return api.NodeShellSession{}, fmt.Errorf("%w: node shells are not wired up", api.ErrInternal)
	}
	return m.nodeShells.Start(ctx, node)
}

func (m *Manager) RemoveNodeShell(ctx context.Context, pod string) error {
	if m.nodeShells == nil {
		return nil
	}
	return m.nodeShells.Remove(ctx, pod)
}
