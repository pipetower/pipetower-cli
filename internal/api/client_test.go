package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pipetower/pipetower-cli/internal/fault"
)

func TestGetSendsAuthAndDecodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" || !strings.HasPrefix(r.Header.Get("User-Agent"), "pt/") {
			t.Errorf("headers = %v", r.Header)
		}
		if r.URL.Path != "/api/v1/servers" || r.URL.Query().Get("search") != "prod" || r.URL.Query().Get("limit") != "100" {
			t.Errorf("url = %s", r.URL)
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"x1","name":"prod","connected":true}],"meta":{"has_more":false}}`))
	}))
	defer server.Close()

	page, err := Get[Page[Server]](context.Background(), New(server.URL+"/", "secret"), List(Servers, ListOptions{Search: "prod", Limit: 100}))
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Data) != 1 || page.Data[0].ID != "x1" || !page.Data[0].Connected {
		t.Errorf("page = %+v", page)
	}
}

func TestErrorsAreExplained(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": "The given data was invalid.",
			"errors":  map[string][]string{"variables.branch": {"The branch is required."}},
		})
	}))
	defer server.Close()

	_, err := New(server.URL, "t").StartRun(context.Background(), RunRequest{ActionID: "a"})

	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Status != 422 {
		t.Fatalf("err = %v", err)
	}
	if got := apiErr.FieldErrors(); len(got) != 1 || !strings.Contains(got[0], "branch is required") {
		t.Errorf("fields = %v", got)
	}
	if fault.ExitCode(err) != fault.ExitAPI {
		t.Errorf("exit code = %d", fault.ExitCode(err))
	}
}

func TestUnauthorizedIsConfigError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := New(server.URL, "t").Account(context.Background())

	if fault.ExitCode(err) != fault.ExitConfig {
		t.Errorf("exit code = %d for %v", fault.ExitCode(err), err)
	}
}

func TestUnreachableInstance(t *testing.T) {
	_, err := New("http://127.0.0.1:1", "t").Account(context.Background())

	var connErr *ConnectionError
	if !errors.As(err, &connErr) {
		t.Fatalf("err = %v", err)
	}
}

func TestListAllFollowsPages(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		more := r.URL.Query().Get("page") == ""
		_ = json.NewEncoder(w).Encode(Page[Server]{Data: []Server{{ID: r.URL.Query().Get("page")}}, Meta: Meta{HasMore: more}})
	}))
	defer server.Close()

	items, err := ListAll[Server](context.Background(), New(server.URL, "t"), Servers, ListOptions{}, 500)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(items) != 2 {
		t.Errorf("calls = %d, items = %v", calls, items)
	}
}
