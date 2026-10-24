package hitokoto

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizeURL(t *testing.T) {
	got, err := NormalizeURL("")
	if err != nil || got != DefaultAPIURL {
		t.Fatalf("default URL = %q, %v", got, err)
	}
	got, err = NormalizeURL("hitokotogo.yhdzz.cn/v2")
	if err != nil || got != "https://hitokotogo.yhdzz.cn/v2" {
		t.Fatalf("scheme-less URL = %q, %v", got, err)
	}
	if _, err := NormalizeURL("ftp://example.com"); err == nil {
		t.Fatal("ftp URL should be rejected")
	}
	if _, err := NormalizeURL("https://"); err == nil {
		t.Fatal("URL without host should be rejected")
	}
}

func TestFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":1,"uuid":"x","hitokoto":"  生活不止眼前的苟且。  ","type":"g","from":"生活不止眼前的苟且","from_who":"许巍","length":12}`)
	}))
	defer server.Close()

	view, err := Fetch(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if view.Text != "生活不止眼前的苟且。" || view.From != "生活不止眼前的苟且" || view.FromWho != "许巍" || view.Type != "g" {
		t.Fatalf("unexpected view: %#v", view)
	}
}

func TestFetchEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"hitokoto":""}`)
	}))
	defer server.Close()

	if _, err := Fetch(context.Background(), server.URL); err == nil {
		t.Fatal("empty sentence should fail")
	}
}
