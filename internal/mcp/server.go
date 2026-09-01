package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Options struct {
	Version    string
	Context    string
	Protected  func() bool
	AllowWrite bool
	Prometheus Prometheus
	LogLines   int
	CallBudget time.Duration
}

const (
	defaultLogLines   = 200
	defaultCallBudget = 60 * time.Second
)

type Server struct {
	cluster    Cluster
	prom       Prometheus
	tools      map[string]tool
	version    string
	context    string
	protected  func() bool
	allowWrite bool
	logLines   int
	budget     time.Duration
}

func New(cluster Cluster, opts Options) *Server {
	lines := opts.LogLines
	if lines == 0 {
		lines = defaultLogLines
	}
	budget := opts.CallBudget
	if budget == 0 {
		budget = defaultCallBudget
	}
	server := &Server{
		cluster:    cluster,
		prom:       opts.Prometheus,
		tools:      map[string]tool{},
		version:    opts.Version,
		context:    opts.Context,
		protected:  opts.Protected,
		allowWrite: opts.AllowWrite,
		logLines:   lines,
		budget:     budget,
	}
	server.registerReads()
	server.registerWrites()
	return server
}

func (s *Server) isProtected() bool {
	if s.protected == nil {
		return false
	}
	return s.protected()
}

func (s *Server) writesAllowed() bool {
	if !s.allowWrite {
		return false
	}
	return !s.isProtected()
}

func (s *Server) toolFor(name string) (tool, bool) {
	one, known := s.tools[name]
	if !known {
		return tool{}, false
	}
	if one.writes && !s.writesAllowed() {
		return tool{}, false
	}
	return one, true
}

func (s *Server) Tools() []string {
	return toolNames(s.cards())
}

func (s *Server) Handle(ctx context.Context, raw []byte) []byte {
	var call request
	if err := json.Unmarshal(raw, &call); err != nil {
		return encode(refuse(nil, codeParse, "the message is not JSON"))
	}
	if call.JSONRPC != jsonRPCVersion {
		return encode(refuse(call.ID, codeInvalidRequest, "jsonrpc must be "+jsonRPCVersion))
	}
	if len(call.ID) == 0 {
		s.notified(call.Method)
		return nil
	}
	return encode(s.dispatch(ctx, call))
}

func (s *Server) notified(method string) {
	_ = method
}

func (s *Server) dispatch(ctx context.Context, call request) *response {
	switch call.Method {
	case "initialize":
		return answer(call.ID, s.hello())
	case "ping":
		return answer(call.ID, map[string]any{})
	case "tools/list":
		return answer(call.ID, toolListing{Tools: s.cards()})
	case "tools/call":
		return s.callTool(ctx, call)
	case "resources/list":
		return answer(call.ID, resourceListing{Resources: resourceCards()})
	case "resources/read":
		return s.readResource(ctx, call)
	default:
		return refuse(call.ID, codeMethodNotFound, "spinoza serves no method called "+call.Method)
	}
}

func (s *Server) hello() initializeResult {
	return initializeResult{
		ProtocolVersion: protocolVersion,
		Capabilities: capabilities{
			Tools:     &listing{},
			Resources: &listing{},
		},
		ServerInfo:   serverInfo{Name: serverName, Title: "Spinoza", Version: s.version},
		Instructions: s.instructions(),
	}
}

func (s *Server) instructions() string {
	lines := []string{
		"Reads one Kubernetes cluster through spinoza's own informer caches.",
		"Context: " + s.context + ".",
	}
	if s.isProtected() {
		lines = append(lines, "This cluster is marked protected in spinoza, so every write tool is withheld.")
	}
	if !s.writesAllowed() {
		lines = append(lines, "Read-only: no tool here can change the cluster.")
	}
	lines = append(lines, "Secret values are never returned; key names and sizes are.")
	return strings.Join(lines, " ")
}

func (s *Server) callTool(ctx context.Context, call request) *response {
	var params callParams
	if err := json.Unmarshal(call.Params, &params); err != nil {
		return refuse(call.ID, codeInvalidParams, "the call parameters are not an object")
	}
	found, known := s.toolFor(params.Name)
	if !known {
		return refuse(call.ID, codeInvalidParams, s.unknown(params.Name))
	}
	result, err := s.runBounded(ctx, found, arguments(params.Arguments))
	if err != nil {
		return answer(call.ID, callResult{Content: []content{{Type: "text", Text: err.Error()}}, IsError: true})
	}
	body, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return refuse(call.ID, codeInternal, "the result could not be encoded: "+err.Error())
	}
	return answer(call.ID, callResult{Content: []content{{Type: "text", Text: string(body)}}})
}

func (s *Server) runBounded(ctx context.Context, one tool, args arguments) (any, error) {
	bounded, cancel := context.WithTimeout(ctx, s.budget)
	defer cancel()
	result, err := one.run(bounded, args)
	if err != nil {
		return nil, err
	}
	if bounded.Err() != nil {
		return nil, fmt.Errorf("%s did not finish within %s", one.name, s.budget)
	}
	return result, nil
}

func (s *Server) unknown(name string) string {
	if !s.writesAllowed() && writeToolNames[name] {
		return name + " changes the cluster and this server is read-only"
	}
	return "spinoza serves no tool called " + name
}

func encode(reply *response) []byte {
	if reply == nil {
		return nil
	}
	body, err := json.Marshal(reply)
	if err != nil {
		return fmt.Appendf(
			nil,
			`{"jsonrpc":%q,"error":{"code":%d,"message":"the reply could not be encoded"}}`,
			jsonRPCVersion, codeInternal,
		)
	}
	return body
}
