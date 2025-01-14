/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package httplog

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"k8s.io/apiserver/pkg/endpoints/responsewriter"
)

func TestDefaultStacktracePred(t *testing.T) {
	for _, x := range []int{101, 200, 204, 302, 400, 404} {
		if DefaultStacktracePred(x) {
			t.Fatalf("should not log on %v by default", x)
		}
	}

	for _, x := range []int{500, 100} {
		if !DefaultStacktracePred(x) {
			t.Fatalf("should log on %v by default", x)
		}
	}
}

func TestStatusIsNot(t *testing.T) {
	statusTestTable := []struct {
		status   int
		statuses []int
		want     bool
	}{
		{http.StatusOK, []int{}, true},
		{http.StatusOK, []int{http.StatusOK}, false},
		{http.StatusCreated, []int{http.StatusOK, http.StatusAccepted}, true},
	}
	for _, tt := range statusTestTable {
		sp := StatusIsNot(tt.statuses...)
		got := sp(tt.status)
		if got != tt.want {
			t.Errorf("Expected %v, got %v", tt.want, got)
		}
	}
}

func TestWithLogging(t *testing.T) {
	req, err := http.NewRequest("GET", "http://example.com", nil)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	var handler http.Handler
	handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	handler = WithLogging(WithLogging(handler, DefaultStacktracePred), DefaultStacktracePred)

	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("Expected newLogged to panic")
			}
		}()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}()
}

func TestLogOf(t *testing.T) {
	logOfTests := []bool{true, false}
	for _, makeLogger := range logOfTests {
		req, err := http.NewRequest("GET", "http://example.com", nil)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		var want string
		var handler http.Handler
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := reflect.TypeOf(LogOf(r, w)).String()
			if want != got {
				t.Errorf("Expected %v, got %v", want, got)
			}
		})
		if makeLogger {
			handler = WithLogging(handler, DefaultStacktracePred)
			want = "*httplog.respLogger"
		} else {
			want = "*httplog.passthroughLogger"
		}

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}
}

func TestUnlogged(t *testing.T) {
	unloggedTests := []bool{true, false}
	for _, makeLogger := range unloggedTests {
		req, err := http.NewRequest("GET", "http://example.com", nil)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		origWriter := httptest.NewRecorder()
		var handler http.Handler
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := Unlogged(r, w)
			if origWriter != got {
				t.Errorf("Expected origin writer, got %#v", got)
			}
		})
		if makeLogger {
			handler = WithLogging(handler, DefaultStacktracePred)
		}

		handler.ServeHTTP(origWriter, req)
	}
}

func TestLoggedStatus(t *testing.T) {
	req, err := http.NewRequest("GET", "http://example.com", nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	var tw http.ResponseWriter = new(responsewriter.FakeResponseWriter)
	logger, _ := newLogged(req, tw)
	logger.Write(nil)

	if logger.status != http.StatusOK {
		t.Errorf("expected status after write to be %v, got %v", http.StatusOK, logger.status)
	}

	tw = new(responsewriter.FakeResponseWriter)
	logger, _ = newLogged(req, tw)
	logger.WriteHeader(http.StatusForbidden)
	logger.Write(nil)

	if logger.status != http.StatusForbidden {
		t.Errorf("expected status after write to remain %v, got %v", http.StatusForbidden, logger.status)
	}
}

func TestRespLoggerWithDecoratedResponseWriter(t *testing.T) {
	req, err := http.NewRequest("GET", "http://example.com", nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	var tw http.ResponseWriter = new(responsewriter.FakeResponseWriter)
	_, rwGot := newLogged(req, tw)

	switch v := rwGot.(type) {
	case *respLogger:
	default:
		t.Errorf("Expected respLogger, got %v", reflect.TypeOf(v))
	}

	tw = new(responsewriter.FakeResponseWriterFlusherCloseNotifier)
	_, rwGot = newLogged(req, tw)

	//lint:file-ignore SA1019 Keep supporting deprecated http.CloseNotifier
	if _, ok := rwGot.(http.CloseNotifier); !ok {
		t.Errorf("Expected http.ResponseWriter to implement http.CloseNotifier")
	}
	if _, ok := rwGot.(http.Flusher); !ok {
		t.Errorf("Expected the wrapper to implement http.Flusher")
	}
	if _, ok := rwGot.(http.Hijacker); ok {
		t.Errorf("Expected http.ResponseWriter not to implement http.Hijacker")
	}

	tw = new(responsewriter.FakeResponseWriterFlusherCloseNotifierHijacker)
	_, rwGot = newLogged(req, tw)

	//lint:file-ignore SA1019 Keep supporting deprecated http.CloseNotifier
	if _, ok := rwGot.(http.CloseNotifier); !ok {
		t.Errorf("Expected http.ResponseWriter to implement http.CloseNotifier")
	}
	if _, ok := rwGot.(http.Flusher); !ok {
		t.Errorf("Expected the wrapper to implement http.Flusher")
	}
	if _, ok := rwGot.(http.Hijacker); !ok {
		t.Errorf("Expected http.ResponseWriter to implement http.Hijacker")
	}
}

func TestResponseWriterDecorator(t *testing.T) {
	decorator := &respLogger{
		w: &responsewriter.FakeResponseWriter{},
	}
	var w http.ResponseWriter = decorator

	if inner := w.(responsewriter.UserProvidedDecorator).Unwrap(); inner != decorator.w {
		t.Errorf("Expected the decorator to return the inner http.ResponseWriter object")
	}
}
