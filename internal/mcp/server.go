package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type Options struct {
	Version    string
	Context    string
	Protected  bool
	AllowWrite bool
	Prometheus Prometheus
	LogLines   int
}

const defaultLogLines = 200

type Server struct {
	cluster     Cluster
	prom        Prometheus
	tools       map[string]tool
	version     string
	context     string
	protected   bool
	allowWrites bool
	logLines    int
}

func New(cluster Cluster, opts Options) *Server {
	lines := opts.LogLines
	if lines == 0 {
		lines = defaultLogLines
	}
	server := &Server{
		cluster:     cluster,
		prom:        opts.Prometheus,
		tools:       map[string]tool{},
		version:     opts.Version,
		context:     opts.Context,
		protected:   opts.Protected,
		allowWrites: opts.AllowWrite && !opts.Protected,
		logLines:    lines,
	}
	server.registerReads()
	server.registerWrites()
	return server
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
	if s.protected {
		lines = append(lines, "This cluster is marked protected in spinoza, so every write tool is withheld.")
	}
	if !s.allowWrites {
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
	found, known := s.tools[params.Name]
	if !known {
		return refuse(call.ID, codeInvalidParams, s.unknown(params.Name))
	}
	result, err := found.run(ctx, arguments(params.Arguments))
	if err != nil {
		return answer(call.ID, callResult{Content: []content{{Type: "text", Text: err.Error()}}, IsError: true})
	}
	body, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return refuse(call.ID, codeInternal, "the result could not be encoded: "+err.Error())
	}
	return answer(call.ID, callResult{Content: []content{{Type: "text", Text: string(body)}}})
}

func (s *Server) unknown(name string) string {
	if !s.allowWrites && writeToolNames[name] {
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
