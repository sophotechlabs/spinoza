package mcp

import (
	"context"
	"encoding/json"
	"fmt"
)

const (
	healthURI   = "cluster://health"
	topologyURI = "cluster://topology"
	jsonMime    = "application/json"
)

func resourceCards() []resourceCard {
	return []resourceCard{
		{URI: healthURI, Name: "Cluster health", Description: "Versions, node and pod counts, and what is failing now.", MimeType: jsonMime},
		{URI: topologyURI, Name: "Cluster topology", Description: "The folded ownership and wiring graph.", MimeType: jsonMime},
	}
}

func (s *Server) readResource(ctx context.Context, call request) *response {
	var params readParams
	if err := json.Unmarshal(call.Params, &params); err != nil {
		return refuse(call.ID, codeInvalidParams, "the read parameters are not an object")
	}
	body, err := s.resourceBody(ctx, params.URI)
	if err != nil {
		return refuse(call.ID, codeInvalidParams, err.Error())
	}
	encoded, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return refuse(call.ID, codeInternal, "the resource could not be encoded: "+err.Error())
	}
	return answer(call.ID, readResult{Contents: []resourceBody{{
		URI:      params.URI,
		MimeType: jsonMime,
		Text:     string(encoded),
	}}})
}

func (s *Server) resourceBody(ctx context.Context, uri string) (any, error) {
	switch uri {
	case healthURI:
		return s.dashboard(ctx, arguments{})
	case topologyURI:
		return s.topology(ctx, arguments{})
	default:
		return nil, fmt.Errorf("spinoza serves no resource at %s", uri)
	}
}
