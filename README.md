# mcp-developer-overheid-api-register

This project provides a [Model Context
Protocol](https://modelcontextprotocol.io/) (MCP) server for the [Developer
Overheid
API](https://apis.developer.overheid.nl/apis/minbzk-developer-overheid). It
allows AI tools to interact with the Developer Overheid API Register through MCP
tools.

## Features

- Implements a [Model Context Protocol](https://modelcontextprotocol.io/) (MCP) server
- Provides tools for interacting with the Developer Overheid API:
  - `list_apis`: List all APIs exposed via the Developer Overheid API
  - `get_api`: Get API details by ID
  - `list_repositories`: List all CVS repositories

## Requirements

- [Go 1.24](https://go.dev/dl/)

## Installation

Configuration for common MCP hosts (Claude Desktop, Cursor):

```json
{
  "mcpServers": {
    "developer-overheid-api-register": {
      "command": "go",
      "args": [
        "run",
        "github.com/dstotijn/mcp-developer-overheid-api-register@main"
      ]
    }
  }
}
```

Alternatively, you can manually install the program (given you have Go installed):

```sh
go install github.com/dstotijn/mcp-developer-overheid-api-register@main
```

## Usage

```
$ mcp-developer-overheid-api-register --help

Usage of mcp-developer-overheid-api-register:
  -http string
        HTTP listen address for JSON-RPC over HTTP (default ":8080")
  -sse
        Enable SSE transport
  -stdio
        Enable stdio transport (default true)
```

Typically, your MCP host will run the program and start the MCP server, and you
don't need to manually do this. But if you want to run the MCP server manually,
for instance because you want to serve over HTTP (using SSE):

Given you have your `PATH` environment configured to include the path named by
`$GOBIN` (or `$GOPATH/bin` `$HOME/go/bin` if `$GOBIN` is not set), you can then
run:

```sh
mcp-developer-overheid-api-register --stdio=false --sse
```

Which will output the SSE transport URL:

```
2025/03/12 15:20:01 MCP server started, using transports: [sse]
2025/03/12 15:20:01 SSE transport endpoint: http://localhost:8080
```

## License

[Apache-2.0 license](/LICENSE)

---

©️ 2025 David Stotijn
