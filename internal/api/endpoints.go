package api

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// Resource names of the list and show endpoints.
const (
	Servers   = "servers"
	Actions   = "actions"
	Workflows = "workflows"
	Pipes     = "pipes"
	Templates = "templates"
	Teams     = "teams"
	Runs      = "runs"
)

// MaxLimit is the largest page size the API accepts.
const MaxLimit = 100

// ListOptions filters and paginates a list endpoint.
type ListOptions struct {
	Search string
	Team   string
	Limit  int
	Page   int
	Params map[string]string
}

// query encodes the options as URL parameters.
func (o ListOptions) query() url.Values {
	q := url.Values{}
	if o.Search != "" {
		q.Set("search", o.Search)
	}
	if o.Team != "" {
		q.Set("team", o.Team)
	}
	if o.Limit > 0 {
		q.Set("limit", strconv.Itoa(o.Limit))
	}
	if o.Page > 1 {
		q.Set("page", strconv.Itoa(o.Page))
	}
	for key, value := range o.Params {
		if value != "" {
			q.Set(key, value)
		}
	}
	return q
}

// List is the endpoint listing a resource, path may be nested like "servers/x/runs".
func List(path string, opts ListOptions) Endpoint {
	return Endpoint{Method: http.MethodGet, Path: path, Query: opts.query()}
}

// Show is the endpoint of a single resource.
func Show(resource, id string) Endpoint {
	return Endpoint{Method: http.MethodGet, Path: resource + "/" + url.PathEscape(id)}
}

// NestedPath is the path of a list below a resource.
func NestedPath(resource, id, sub string) string {
	return resource + "/" + url.PathEscape(id) + "/" + sub
}

// Post is a POST endpoint with an optional JSON body.
func Post(path string, body any) Endpoint {
	return Endpoint{Method: http.MethodPost, Path: path, Body: body}
}

// Put is a PUT endpoint with a JSON body.
func Put(path string, body any) Endpoint {
	return Endpoint{Method: http.MethodPut, Path: path, Body: body}
}

// Delete is the endpoint deleting a single resource.
func Delete(resource, id string) Endpoint {
	return Endpoint{Method: http.MethodDelete, Path: resource + "/" + url.PathEscape(id)}
}

// Definition is the endpoint of the definition of a resource: the portable description an export file contains.
func Definition(resource, id string) Endpoint {
	return Endpoint{Method: http.MethodGet, Path: resource + "/" + url.PathEscape(id) + "/definition"}
}

// Example is the endpoint of an example definition of a kind which uses every option: as JSON,
// or as YAML with comments which explain the keys.
func Example(kind string, asYAML bool) Endpoint {
	e := Endpoint{Method: http.MethodGet, Path: "definitions/" + url.PathEscape(kind) + "/example"}
	if asYAML {
		e.Query = url.Values{"format": {"yaml"}}
	}
	return e
}

// RunEndpoint is the endpoint of a run, optionally with the output of all steps.
func RunEndpoint(id string, withOutput bool) Endpoint {
	e := Show(Runs, id)
	if withOutput {
		e.Query = url.Values{"include": {"output"}}
	}
	return e
}

// ExecutionLogEndpoint is the text export of a run execution.
func ExecutionLogEndpoint(id string) Endpoint {
	return Endpoint{Method: http.MethodGet, Path: "run-executions/" + url.PathEscape(id) + "/log"}
}

// Account returns the authenticated user.
func (c *Client) Account(ctx context.Context) (Account, error) {
	return Get[Account](ctx, c, Endpoint{Method: http.MethodGet, Path: "account"})
}

// Run returns a run, optionally with the output of its steps.
func (c *Client) Run(ctx context.Context, id string, withOutput bool) (Run, error) {
	return Get[Run](ctx, c, RunEndpoint(id, withOutput))
}

// StartRun starts an action or workflow on servers.
func (c *Client) StartRun(ctx context.Context, req RunRequest) (Run, error) {
	return Get[Run](ctx, c, Post(Runs, req))
}

// TerminateRun terminates all executions of a run.
func (c *Client) TerminateRun(ctx context.Context, id string) (Run, error) {
	return Get[Run](ctx, c, Post("runs/"+url.PathEscape(id)+"/terminate", nil))
}

// Server returns a server with its latest metrics.
func (c *Client) Server(ctx context.Context, id string) (Server, error) {
	return Get[Server](ctx, c, Show(Servers, id))
}

// SetConnected connects or disconnects the SSH connection of a server.
func (c *Client) SetConnected(ctx context.Context, id string, connected bool) (Server, error) {
	action := "disconnect"
	if connected {
		action = "connect"
	}
	return Get[Server](ctx, c, Post("servers/"+url.PathEscape(id)+"/"+action, nil))
}

// GatewayTicket issues a single-use ticket for a gateway connection.
func (c *Client) GatewayTicket(ctx context.Context) (Ticket, error) {
	return Get[Ticket](ctx, c, Post("gateway/ticket", nil))
}

// OpenSession opens a new terminal session on a server.
func (c *Client) OpenSession(ctx context.Context, serverID string) (Session, error) {
	return Get[Session](ctx, c, Post("servers/"+url.PathEscape(serverID)+"/terminal-sessions", nil))
}

// AttachSession issues a fresh attach ticket for a terminal session.
func (c *Client) AttachSession(ctx context.Context, sessionID string) (Session, error) {
	return Get[Session](ctx, c, Post("terminal-sessions/"+url.PathEscape(sessionID)+"/attach", nil))
}

// ListAll collects all pages of a list endpoint, up to maxItems.
func ListAll[T any](ctx context.Context, c *Client, path string, opts ListOptions, maxItems int) ([]T, error) {
	opts.Limit, opts.Page = MaxLimit, 1
	var items []T

	for {
		page, err := Get[Page[T]](ctx, c, List(path, opts))
		if err != nil {
			return nil, err
		}
		items = append(items, page.Data...)
		if len(items) >= maxItems {
			return items[:maxItems], nil
		}
		if !page.Meta.HasMore || len(page.Data) == 0 {
			return items, nil
		}
		opts.Page++
	}
}
