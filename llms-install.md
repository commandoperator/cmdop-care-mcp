# cmdop-care MCP install notes for AI clients

`cmdop-care` is a local stdio MCP server. It exposes four read-only tools for
an already-enrolled CMDOP machine fleet:

- `list_machines`
- `care_status`
- `care_diagnose`
- `care_storage_inventory`

It does not run shell commands, read arbitrary files, list directories, or
delegate open-ended work to another machine.

## Prerequisite

Install and enroll the main CMDOP CLI on the host first. `cmdop-care` reads the
existing enrollment token from the host OS keyring. It does not accept a relay
token as an MCP config value.

## Build from source

```bash
git clone https://github.com/commandoperator/cmdop-care-mcp.git
cd cmdop-care-mcp
go build -o cmdop-care .
```

## MCP client configuration

Use the built binary as a stdio MCP server:

```json
{
  "mcpServers": {
    "cmdop-care": {
      "command": "/absolute/path/to/cmdop-care-mcp/cmdop-care",
      "args": []
    }
  }
}
```

Replace `/absolute/path/to/cmdop-care-mcp/cmdop-care` with the path to the
binary you built.

## First prompt

After connecting the server, ask:

```text
List my enrolled CMDOP machines.
```

The assistant should call `list_machines` first. If the host is not enrolled,
the tool returns a clear not-enrolled message instead of silently failing.

## Security boundary

`cmdop-care` is intentionally narrower than the full CMDOP MCP surface. Use it
when you want an assistant to inspect fleet health and CMDOP-owned storage
without exposing command execution, arbitrary filesystem access, or open-ended
machine delegation.
