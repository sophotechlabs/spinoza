package mcp

import (
	"context"
	"slices"
)

type tool struct {
	name        string
	title       string
	description string
	properties  map[string]propOf
	required    []string
	writes      bool
	destructive bool
	idempotent  bool
	run         func(ctx context.Context, args arguments) (any, error)
}

func (s *Server) cards() []toolCard {
	names := make([]string, 0, len(s.tools))
	for name := range s.tools {
		names = append(names, name)
	}
	slices.Sort(names)
	out := make([]toolCard, 0, len(names))
	for _, name := range names {
		one, served := s.toolFor(name)
		if !served {
			continue
		}
		out = append(out, one.card())
	}
	return out
}

func (t tool) card() toolCard {
	properties := t.properties
	if properties == nil {
		properties = map[string]propOf{}
	}
	return toolCard{
		Name:        t.name,
		Title:       t.title,
		Description: t.description,
		InputSchema: schema{Type: "object", Properties: properties, Required: t.required},
		Annotations: annotations{
			Title:           t.title,
			ReadOnlyHint:    !t.writes,
			DestructiveHint: t.destructive,
			IdempotentHint:  t.idempotent,
		},
	}
}

func text(description string) propOf {
	return propOf{Type: "string", Description: description}
}

func choice(description string, allowed ...string) propOf {
	return propOf{Type: "string", Description: description, Enum: allowed}
}

func number(description string) propOf {
	return propOf{Type: "integer", Description: description}
}

func toggle(description string) propOf {
	return propOf{Type: "boolean", Description: description}
}

func (s *Server) register(one tool) {
	s.tools[one.name] = one
}

func toolNames(cards []toolCard) []string {
	out := make([]string, 0, len(cards))
	for _, card := range cards {
		out = append(out, card.Name)
	}
	return out
}
