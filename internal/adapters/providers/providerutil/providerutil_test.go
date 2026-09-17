package providerutil

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"tavernagent/internal/ports"
)

func TestEmptyStreamErrorClassification(t *testing.T) {
	if err := EmptyStreamError(true); !errors.Is(err, ports.ErrTruncatedStream) {
		t.Fatalf("eventSeen=true: want ErrTruncatedStream, got %v", err)
	}
	if err := EmptyStreamError(false); !errors.Is(err, ports.ErrNonStreamingResponse) {
		t.Fatalf("eventSeen=false: want ErrNonStreamingResponse, got %v", err)
	}
}

func TestStreamAbortDistinguishesCancelFromTimeout(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := StreamAbort(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled ctx: want context.Canceled, got %v", err)
	}
	if err := StreamAbort(context.Background()); !errors.Is(err, ErrTimeoutTotal) {
		t.Fatalf("live ctx: want ErrTimeoutTotal, got %v", err)
	}
}

func TestReadHTTPErrorParsesOpenAIEnvelope(t *testing.T) {
	resp := &http.Response{
		StatusCode: 429,
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"rate limited","type":"rate_limit_error","code":"rate_limit"}}`)),
	}
	err := ReadHTTPError(resp, DecodeOpenAIError)
	var he *HTTPError
	if !errors.As(err, &he) {
		t.Fatalf("want *HTTPError, got %T %v", err, err)
	}
	if he.StatusCode != 429 || he.Code != "rate_limit" || he.Message != "rate limited" {
		t.Fatalf("HTTPError = %+v", he)
	}
	if !strings.Contains(he.Error(), "rate_limit") {
		t.Fatalf("Error() should include code: %q", he.Error())
	}
}

func TestReadHTTPErrorFallsBackToPlainText(t *testing.T) {
	resp := &http.Response{
		StatusCode: 502,
		Body:       io.NopCloser(strings.NewReader("  upstream bad gateway  ")),
	}
	var he *HTTPError
	if err := ReadHTTPError(resp, DecodeOpenAIError); !errors.As(err, &he) {
		t.Fatalf("want *HTTPError, got %T %v", err, err)
	}
	if he.Code != "" || he.Message != "upstream bad gateway" {
		t.Fatalf("HTTPError = %+v", he)
	}
}

func TestReadHTTPErrorEmptyBody(t *testing.T) {
	resp := &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader(""))}
	var he *HTTPError
	if err := ReadHTTPError(resp, DecodeOpenAIError); !errors.As(err, &he) {
		t.Fatalf("want *HTTPError, got %T %v", err, err)
	}
	if he.Message != "无响应体" {
		t.Fatalf("empty body message = %q", he.Message)
	}
}

func TestDecodeOpenAIErrorRejectsNonMatching(t *testing.T) {
	if _, _, ok := DecodeOpenAIError([]byte("not json")); ok {
		t.Fatal("plain text must not decode")
	}
	if _, _, ok := DecodeOpenAIError([]byte(`{"error":{}}`)); ok {
		t.Fatal("empty message must not decode")
	}
}
