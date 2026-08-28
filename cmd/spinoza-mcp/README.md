# spinoza-mcp

An MCP server over one Kubernetes cluster, reading through spinoza's own informer caches.

It is a separate binary. Nothing in the spinoza app knows it exists: it consumes
`internal/cluster` exactly as `main.go` does, and adds no route, flag or type to
the app itself.

## Build

```
go build -o spinoza-mcp ./cmd/spinoza-mcp
```

## Use it as a command

```
spinoza-mcp tools                                   # what it offers
spinoza-mcp call get_dashboard                      # run one tool
spinoza-mcp call get_resource resource=deployments name=web namespace=prod
spinoza-mcp -context p-mk1 call get_issues limit=5
```

The tool definitions are the same objects the MCP server exposes, so the command
line and the protocol cannot drift.

## Use it from an MCP client

It speaks MCP over stdin and stdout, which is what local clients expect.

Claude Code:

```
claude mcp add spinoza -- /path/to/spinoza-mcp -context p-mk1
```

Claude Desktop, Cursor, Windsurf, Cline and VS Code Copilot all take the same
shape in their own config file:

```json
{
  "mcpServers": {
    "spinoza": {
      "command": "/path/to/spinoza-mcp",
      "args": ["-context", "p-mk1"]
    }
  }
}
```

## What it will not do

- **Secret values never leave the process.** A Secret comes back as key names,
  sizes and whether each value is binary. The raw document is withheld entirely.
- **Everything else is scrubbed on the way out**: bearer tokens, JWTs, private
  key blocks, long base64 blobs, `key=value` pairs whose key looks like a
  credential, and env pairs in YAML where the name is on one line and the value
  on the next.
- **Writes are off unless asked for.** `-allow-write` adds five tools that
  change the cluster. Without it they are not offered at all, and asking for one
  says why rather than pretending it does not exist.
- **A protected cluster refuses writes even with `-allow-write`.** spinoza's own
  protection store decides, and its confirm-by-name step cannot be meaningfully
  driven by a model.
- **RBAC still decides.** Every write asks spinoza's access check first and
  returns the apiserver's own reason when refused.

## Flags

| Flag | What it does |
|---|---|
| `-context` | Which kubeconfig context to read. The current one when empty |
| `-kubeconfig` | Which kubeconfig. The usual lookup rules when empty |
| `-allow-write` | Offer the five tools that change the cluster |
| `-prometheus` | `namespace/service:port`. Discovered when empty |
| `-log-lines` | Most log lines any one tool returns. Defaults to 200 |
| `-sync-timeout` | How long to wait for an informer cache |
