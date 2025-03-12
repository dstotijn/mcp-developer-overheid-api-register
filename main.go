// Package main provides an MCP server for the Developer Overheid API.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dstotijn/go-mcp"
)

// Base URL for the Developer Overheid API.
const apiBaseURL = "https://apis.developer.overheid.nl/api/v0"

// ListAPIsParams represents the parameters for the listAPIs tool.
// The `page` parameter is optional.
type ListAPIsParams struct {
	Page int `json:"page,omitempty"`
}

// ListAPIsResponse represents the response from the listAPIs tool.
type ListAPIsResponse struct {
	APIs     json.RawMessage `json:"apis"`
	NextPage int             `json:"next_page,omitempty"`
}

// GetAPIParams represents the parameters for the getAPI tool.
// The `id` parameter is required.
type GetAPIParams struct {
	ID string `json:"id"`
}

// ListRepositoriesParams represents the parameters for the listRepositories tool.
// The `page` parameter is optional.
type ListRepositoriesParams struct {
	Page int `json:"page,omitempty"`
}

// ListRepositoriesResponse represents the response from the listRepositories tool.
type ListRepositoriesResponse struct {
	Repositories json.RawMessage `json:"repositories"`
	NextPage     int             `json:"next_page,omitempty"`
}

// Command-line flags.
var (
	httpAddr string
	useStdio bool
	useSSE   bool
)

var (
	hateoasLinkRegexp = regexp.MustCompile(`<([^>]+)>`)
	relRegexp         = regexp.MustCompile(`rel="([^"]+)"`)
)

func main() {
	flag.StringVar(&httpAddr, "http", ":8080", "HTTP listen address for JSON-RPC over HTTP")
	flag.BoolVar(&useStdio, "stdio", true, "Enable stdio transport")
	flag.BoolVar(&useSSE, "sse", false, "Enable SSE transport")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	transports := []string{}
	opts := []mcp.ServerOption{}

	if useStdio {
		transports = append(transports, "stdio")
		opts = append(opts, mcp.WithStdioTransport())
	}

	var sseURL url.URL

	if useSSE {
		transports = append(transports, "sse")

		host := "localhost"

		hostPart, port, err := net.SplitHostPort(httpAddr)
		if err != nil {
			log.Fatalf("Failed to split host and port: %v", err)
		}

		if hostPart != "" {
			host = hostPart
		}

		sseURL = url.URL{
			Scheme: "http",
			Host:   host + ":" + port,
		}

		opts = append(opts, mcp.WithSSETransport(sseURL))
	}

	mcpServer := mcp.NewServer(mcp.ServerConfig{}, opts...)

	mcpServer.Start(ctx)

	mcpServer.RegisterTools(
		createListAPIsTool(),
		createGetAPITool(),
		createListRepositoriesTool(),
	)

	httpServer := &http.Server{
		Addr:    httpAddr,
		Handler: mcpServer,
		BaseContext: func(l net.Listener) context.Context {
			return ctx
		},
	}

	if useSSE {
		go func() {
			if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("HTTP server error: %v", err)
			}
		}()
	}

	log.Printf("MCP server started, using transports: %v", transports)
	if useSSE {
		log.Printf("SSE transport endpoint: %v", sseURL.String())
	}

	// Wait for interrupt signal.
	<-ctx.Done()
	// Restore signal, allowing "force quit".
	stop()

	timeout := 5 * time.Second
	cancelContext, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	log.Printf("Shutting down server (waiting %s). Press Ctrl+C to force quit.", timeout)

	var wg sync.WaitGroup

	if useSSE {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := httpServer.Shutdown(cancelContext); err != nil && !errors.Is(err, context.DeadlineExceeded) {
				log.Printf("HTTP server shutdown error: %v", err)
			}
		}()
	}

	wg.Wait()
}

func createListAPIsTool() mcp.Tool {
	return mcp.CreateTool(mcp.ToolDef[ListAPIsParams]{
		Name:        "list_apis",
		Description: "List all APIs from the Developer Overheid API.",
		HandleFunc: func(ctx context.Context, params ListAPIsParams) *mcp.CallToolResult {
			page := params.Page
			if page == 0 {
				page = 1
			}

			apiURL := fmt.Sprintf("%v/apis?page=%d", apiBaseURL, page)

			resp, err := http.Get(apiURL)
			if err != nil {
				return newToolCallErrorResult("Error fetching APIs: %v", err)
			}
			defer resp.Body.Close()

			var apis json.RawMessage
			if err := json.NewDecoder(resp.Body).Decode(&apis); err != nil {
				return newToolCallErrorResult("Error parsing response: %v", err)
			}

			// Create response with APIs and next page info.
			response := ListAPIsResponse{
				APIs: apis,
			}

			// Check for Link header to get the next page URL.
			linkHeader := resp.Header.Get("Link")
			if linkHeader != "" {
				// Parse the Link header to find the "next" relation.
				for _, link := range parseLinkHeader(linkHeader) {
					if link.Rel != "next" {
						continue
					}
					// Extract page number from URL.
					parsedURL, err := url.Parse(link.URL)
					if err == nil {
						if pageStr := parsedURL.Query().Get("page"); pageStr != "" {
							if nextPage, err := strconv.Atoi(pageStr); err == nil {
								response.NextPage = nextPage
							}
						}
					}
					break
				}
			}

			result, err := json.Marshal(response)
			if err != nil {
				return newToolCallErrorResult("Error formatting response: %v", err)
			}

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Text: string(result),
					},
				},
			}
		},
	})
}

type LinkRelation struct {
	URL string
	Rel string
}

func parseLinkHeader(header string) []LinkRelation {
	var links []LinkRelation

	// Split the header by comma to get individual link-value pairs
	for _, link := range strings.Split(header, ",") {
		link = strings.TrimSpace(link)

		// Extract URL and rel attributes using pre-compiled regexps
		urlMatch := hateoasLinkRegexp.FindStringSubmatch(link)
		relMatch := relRegexp.FindStringSubmatch(link)

		if len(urlMatch) > 1 && len(relMatch) > 1 {
			links = append(links, LinkRelation{
				URL: urlMatch[1],
				Rel: relMatch[1],
			})
		}
	}

	return links
}

func newToolCallErrorResult(format string, args ...any) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Text: fmt.Sprintf(format, args...),
			},
		},
		IsError: true,
	}
}

func createGetAPITool() mcp.Tool {
	return mcp.CreateTool(mcp.ToolDef[GetAPIParams]{
		Name:        "get_api",
		Description: "Get a specific API by ID from the Developer Overheid API.",
		HandleFunc: func(ctx context.Context, params GetAPIParams) *mcp.CallToolResult {
			url := fmt.Sprintf("%v/apis/%v", apiBaseURL, params.ID)
			resp, err := http.Get(url)
			if err != nil {
				return newToolCallErrorResult("Error fetching API: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusNotFound {
				return newToolCallErrorResult("API with ID %v not found", params.ID)
			}

			var api json.RawMessage
			if err := json.NewDecoder(resp.Body).Decode(&api); err != nil {
				return newToolCallErrorResult("Error parsing response: %v", err)
			}

			result, err := json.MarshalIndent(api, "", "  ")
			if err != nil {
				return newToolCallErrorResult("Error formatting response: %v", err)
			}

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Text: string(result),
					},
				},
			}
		},
	})
}

// createListRepositoriesTool creates a tool for listing repositories.
func createListRepositoriesTool() mcp.Tool {
	return mcp.CreateTool(mcp.ToolDef[ListRepositoriesParams]{
		Name:        "list_repositories",
		Description: "List all repositories from the Developer Overheid API.",
		HandleFunc: func(ctx context.Context, params ListRepositoriesParams) *mcp.CallToolResult {
			page := params.Page
			if page == 0 {
				page = 1
			}

			apiURL := fmt.Sprintf("%v/repositories?page=%v", apiBaseURL, page)

			resp, err := http.Get(apiURL)
			if err != nil {
				return newToolCallErrorResult("Error fetching repositories: %v", err)
			}
			defer resp.Body.Close()

			var repositories json.RawMessage
			if err := json.NewDecoder(resp.Body).Decode(&repositories); err != nil {
				return newToolCallErrorResult("Error parsing response: %v", err)
			}

			// Create response with repositories and next page info.
			response := ListRepositoriesResponse{
				Repositories: repositories,
			}

			// Check for Link header to get the next page URL.
			linkHeader := resp.Header.Get("Link")
			if linkHeader != "" {
				// Parse the Link header to find the "next" relation
				for _, link := range parseLinkHeader(linkHeader) {
					if link.Rel == "next" {
						// Extract page number from URL
						parsedURL, err := url.Parse(link.URL)
						if err == nil {
							if pageStr := parsedURL.Query().Get("page"); pageStr != "" {
								if nextPage, err := strconv.Atoi(pageStr); err == nil {
									response.NextPage = nextPage
								}
							}
						}
						break
					}
				}
			}

			result, err := json.Marshal(response)
			if err != nil {
				return newToolCallErrorResult("Error formatting response: %v", err)
			}

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Text: string(result),
					},
				},
			}
		},
	})
}
