package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func (s *Server) List(out io.Writer) error {
	for _, card := range s.cards() {
		mark := "read"
		if !card.Annotations.ReadOnlyHint {
			mark = "write"
		}
		if _, err := fmt.Fprintf(out, "%-22s %-6s %s\n", card.Name, mark, card.Description); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) Call(ctx context.Context, out io.Writer, name string, pairs []string) error {
	found, known := s.tools[name]
	if !known {
		return fmt.Errorf("%s", s.unknown(name))
	}
	args, err := parsePairs(pairs)
	if err != nil {
		return err
	}
	result, err := s.runBounded(ctx, found, args)
	if err != nil {
		return err
	}
	body, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(body))
	return err
}

func parsePairs(pairs []string) (arguments, error) {
	args := arguments{}
	for _, pair := range pairs {
		key, value, found := strings.Cut(pair, "=")
		if !found {
			return nil, fmt.Errorf("%q is not key=value", pair)
		}
		args[key] = typed(value)
	}
	return args, nil
}

func typed(value string) any {
	if value == "true" {
		return true
	}
	if value == "false" {
		return false
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return value
	}
	return number
}
