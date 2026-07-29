package provider

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestRuntimeSourceFetchHTTP(t *testing.T) {
	t.Parallel()

	source := &RuntimeSource{
		plainClient: &http.Client{
			Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"version":"3.6.10"}`)),
					Request:    request,
				}, nil
			}),
		},
	}

	got, err := source.Fetch(context.Background(), RuntimeProduct{
		Key:              "loki",
		Env:              "qa",
		Type:             "http",
		Endpoint:         "https://qa.example.test/version",
		Method:           http.MethodGet,
		Auth:             RuntimeAuth{Type: "none"},
		AcceptedStatuses: []int{http.StatusOK},
		VersionField:     "version",
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if got.Env != "qa" || got.Type != "http" || got.Version != "3.6.10" {
		t.Fatalf("Fetch() = %+v", got)
	}
}

func TestRuntimeSourceSelectsMTLSClientByEnvironment(t *testing.T) {
	t.Parallel()

	source := &RuntimeSource{
		mtlsClients: map[string]*http.Client{
			"qa":   clientReturningVersion("qa-version"),
			"prod": clientReturningVersion("prod-version"),
		},
	}

	for _, testCase := range []struct {
		env     string
		version string
	}{
		{env: "qa", version: "qa-version"},
		{env: "prod", version: "prod-version"},
	} {
		got, err := source.Fetch(context.Background(), RuntimeProduct{
			Key:              "sample",
			Env:              testCase.env,
			Type:             "http",
			Endpoint:         "https://runtime.example.test/version",
			Method:           http.MethodGet,
			Auth:             RuntimeAuth{Type: "mtls"},
			AcceptedStatuses: []int{http.StatusOK},
			VersionField:     "version",
		})
		if err != nil {
			t.Fatalf("Fetch(%s) error = %v", testCase.env, err)
		}
		if got.Version != testCase.version {
			t.Fatalf("Fetch(%s) version = %q, want %q", testCase.env, got.Version, testCase.version)
		}
	}
}

func clientReturningVersion(version string) *http.Client {
	return &http.Client{
		Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"version":"` + version + `"}`)),
				Request:    request,
			}, nil
		}),
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}
