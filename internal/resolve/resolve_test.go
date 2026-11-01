package resolve

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/prompt"
)

// fakeAPI serves servers and actions filtered by the search parameter.
func fakeAPI(t *testing.T) *api.Client {
	t.Helper()
	servers := []api.Server{
		{ID: "srv1", Name: "prod", Host: "1.1.1.1", Tags: []api.Tag{{Name: "web"}}},
		{ID: "srv2", Name: "prod-db", Host: "1.1.1.2"},
		{ID: "srv3", Name: "staging", Host: "1.1.1.3", Tags: []api.Tag{{Name: "web"}}},
	}
	actions := []api.Action{{ID: "act1", Name: "Deploy App"}}
	workflows := []api.Workflow{{ID: "wf1", Name: "Deploy All"}}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/")
		search := strings.ToLower(r.URL.Query().Get("search"))
		tag := r.URL.Query().Get("tag")

		switch path {
		case "servers":
			writePage(w, filter(servers, func(s api.Server) bool {
				return strings.Contains(strings.ToLower(s.Name), search) && (tag == "" || hasTag(s, tag))
			}))
		case "actions":
			writePage(w, filter(actions, func(a api.Action) bool { return strings.Contains(strings.ToLower(a.Name), search) }))
		case "workflows":
			writePage(w, filter(workflows, func(wf api.Workflow) bool { return strings.Contains(strings.ToLower(wf.Name), search) }))
		case "servers/srv2":
			_ = json.NewEncoder(w).Encode(servers[1])
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	return api.New(server.URL, "t")
}

func writePage[T any](w http.ResponseWriter, items []T) {
	_ = json.NewEncoder(w).Encode(api.Page[T]{Data: items})
}

func filter[T any](items []T, keep func(T) bool) []T {
	var result []T
	for _, item := range items {
		if keep(item) {
			result = append(result, item)
		}
	}
	return result
}

func hasTag(s api.Server, tag string) bool {
	for _, t := range s.Tags {
		if t.Name == tag {
			return true
		}
	}
	return false
}

// fakePrompter always picks the given index.
type fakePrompter struct {
	pick   int
	asked  []string
	titles []string
}

func (f *fakePrompter) Pick(title string, options []prompt.Option) (int, error) {
	f.titles, f.asked = append(f.titles, title), nil
	for _, option := range options {
		f.asked = append(f.asked, option.Columns[0])
	}
	return f.pick, nil
}
func (f *fakePrompter) PickMany(title string, options []prompt.Option) ([]int, error) {
	index, err := f.Pick(title, options)
	return []int{index}, err
}
func (f *fakePrompter) Input(string) (string, error) { return "", nil }
func (f *fakePrompter) Confirm(string, string, string) (bool, error) {
	return false, nil
}

func TestExactNameWinsOverPartial(t *testing.T) {
	r := &Resolver{Client: fakeAPI(t)}

	server, err := r.Server(context.Background(), "PROD")
	if err != nil || server.ID != "srv1" {
		t.Fatalf("server = %+v, err = %v", server, err)
	}
}

func TestIDLookup(t *testing.T) {
	r := &Resolver{Client: fakeAPI(t)}

	server, err := r.Server(context.Background(), "srv2")
	if err != nil || server.Name != "prod-db" {
		t.Fatalf("server = %+v, err = %v", server, err)
	}
}

func TestPartialMatchSuggestsWithoutPrompter(t *testing.T) {
	r := &Resolver{Client: fakeAPI(t)}

	_, err := r.Server(context.Background(), "stag")

	var notFound *NotFoundError
	if !errors.As(err, &notFound) || len(notFound.Suggestions) != 1 {
		t.Fatalf("err = %v", err)
	}
}

func TestPartialMatchIsPickedInteractively(t *testing.T) {
	prompter := &fakePrompter{pick: 1}
	r := &Resolver{Client: fakeAPI(t), Prompter: prompter}

	server, err := r.Server(context.Background(), "pro")
	if err != nil || server.ID != "srv2" || len(prompter.asked) != 2 {
		t.Fatalf("server = %+v, err = %v, asked = %v", server, err, prompter.asked)
	}
}

func TestServersExpandsTagsAndDeduplicates(t *testing.T) {
	r := &Resolver{Client: fakeAPI(t)}

	servers, err := r.Servers(context.Background(), []string{"prod", "tag:web"})
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 2 || servers[0].ID != "srv1" || servers[1].ID != "srv3" {
		t.Errorf("servers = %+v", servers)
	}
}

func TestUnknownTag(t *testing.T) {
	r := &Resolver{Client: fakeAPI(t)}

	_, err := r.Servers(context.Background(), []string{"tag:nope"})

	var notFound *NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestWithoutArgumentTheUserPicksAmongAll(t *testing.T) {
	prompter := &fakePrompter{pick: 2}
	r := &Resolver{Client: fakeAPI(t), Prompter: prompter}

	server, err := r.Server(context.Background(), "")
	if err != nil || server.ID != "srv3" || len(prompter.asked) != 3 {
		t.Fatalf("server = %+v, err = %v, asked %v", server, err, prompter.asked)
	}
}

func TestWithoutArgumentAndPrompterTheArgumentIsMissing(t *testing.T) {
	r := &Resolver{Client: fakeAPI(t)}

	if _, err := r.Server(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "Missing server") {
		t.Fatalf("err = %v, want the missing server", err)
	}
}
