package r2sql_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ojiverse/datawarehouse/internal/r2sql"
)

type captured struct {
	auth  string
	path  string
	query string
}

func serve(t *testing.T, status int, body string) (*r2sql.Client, *captured) {
	t.Helper()
	cap := &captured{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.auth = r.Header.Get("Authorization")
		cap.path = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		var req map[string]string
		_ = json.Unmarshal(b, &req)
		cap.query = req["query"]
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	cl, err := r2sql.New(r2sql.Config{AccountID: "acct", Bucket: "bkt", Token: "tok", BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	return cl, cap
}

func TestQueryParsesWrappedRows(t *testing.T) {
	cl, _ := serve(t, 200, `{"success":true,"errors":[],"result":{"rows":[{"n":3}]}}`)
	res, err := cl.Query(context.Background(), "SELECT 1")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 1 || res.Rows[0]["n"].(float64) != 3 {
		t.Fatalf("rows %v", res.Rows)
	}
}

func TestQueryParsesBareArrayAndSendsAuth(t *testing.T) {
	cl, cap := serve(t, 200, `[{"message_id":"1"}]`)
	res, err := cl.Query(context.Background(), "SELECT message_id FROM ns.t")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 1 || res.Rows[0]["message_id"] != "1" {
		t.Fatalf("rows %v", res.Rows)
	}
	if cap.auth != "Bearer tok" || cap.path != "/api/v1/accounts/acct/r2-sql/query/bkt" || cap.query != "SELECT message_id FROM ns.t" {
		t.Fatalf("request %+v", *cap)
	}
}

func TestQueryReportsHTTPError(t *testing.T) {
	cl, _ := serve(t, 401, `{"success":false,"errors":[{"code":10000,"message":"Authentication error"}]}`)
	res, err := cl.Query(context.Background(), "SELECT 1")
	if err == nil || res.HTTPStatus != 401 {
		t.Fatalf("expected 401 error, got %v %d", err, res.HTTPStatus)
	}
}
