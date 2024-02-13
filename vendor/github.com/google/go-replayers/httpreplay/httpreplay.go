// Copyright 2019 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package httpreplay provides an API for recording and replaying traffic
// from HTTP-based clients.
//
// To record:
//  1.  Call NewRecorder to get a Recorder.
//  2.  Use its Client method to obtain an HTTP client to use when making API calls.
//  3.  Close the Recorder when you're done. That will save the log of interactions
//      to the file you provided to NewRecorder.
//
// To replay:
//  1.  Call NewReplayer with the same filename you used to record to get a Replayer.
//  2.  Call its Client method and use the client to make the same API calls.
//      You will get back the recorded responses.
//  3.  Close the Replayer when you're done.
//
// Although it has some features specific to Google HTTP APIs, this package can be used
// more generally.
package httpreplay

// TODO(jba): add examples.

import (
	"net/http"
	"net/url"

	"github.com/google/go-replayers/httpreplay/internal/proxy"
)

// A Recorder records HTTP interactions.
type Recorder struct {
	proxy   *proxy.Proxy
	port    int
	initial []byte
	cert    string
	key     string
}

type recorderOption func(*Recorder)

// NewRecorder creates a recorder that writes to filename. The file will
// also store initial state that can be retrieved to configure replay.
//
// You must call Close on the Recorder to ensure that all data is written.
func NewRecorder(filename string, initial []byte) (*Recorder, error) {
	p, err := proxy.ForRecording(filename, 0, "", "")
	if err != nil {
		return nil, err
	}
	p.Initial = initial
	return &Recorder{proxy: p}, nil
}

// The custom CA cert filename to use for the MITM proxy to create certs on the
// fly
func RecorderCert(fileName string) recorderOption {
	return func(r *Recorder) {
		r.cert = fileName
	}
}

// The key filename to the custom CA cert to use for the MITM proxy to create
// certs on the fly
func RecorderKey(fileName string) recorderOption {
	return func(r *Recorder) {
		r.key = fileName
	}
}

// The recorder file will store initial data that can be retrieved during
// replay
func RecorderInitial(initial []byte) recorderOption {
	return func(r *Recorder) {
		r.initial = initial
	}
}

// The port number for the MITM proxy
func RecorderPort(port int) recorderOption {
	return func(r *Recorder) {
		r.port = port
	}
}

// NewRecorderWithOpts creates a recorder that writes to filename.
// The default Recorder MITM proxy can be customised with one or more
// recorderOption.
//
// You must call Close on the Recorder to ensure that all data is written.
func NewRecorderWithOpts(filename string, opts ...recorderOption) (*Recorder, error) {
	r := &Recorder{port: 0, initial: nil, cert: "", key: ""}
	for _, opt := range opts {
		opt(r)
	}
	p, err := proxy.ForRecording(filename, r.port, r.cert, r.key)
	if err != nil {
		return nil, err
	}
	p.Initial = r.initial
	r.proxy = p
	return r, nil
}

// ScrubBody will replace all parts of the request body that match any of the
// provided regular expressions with CLEARED, on both recording and replay.
// Use ScrubBody when the body information is secret or may change from run to
// run.
//
// You may also need to RemoveRequestHeaders for the "Content-Length" header if
// the body length changes from run to run.
//
// Regexps are parsed as regexp.Regexp.
func (r *Recorder) ScrubBody(regexps ...string) {
	r.proxy.ScrubBody(regexps)
}

// RemoveRequestHeaders will remove request headers matching patterns from the log,
// and skip matching them during replay.
//
// Pattern is taken literally except for *, which matches any sequence of characters.
func (r *Recorder) RemoveRequestHeaders(patterns ...string) {
	r.proxy.RemoveRequestHeaders(patterns)
}

// RemoveResponseHeaders will remove response headers matching patterns from the log,
// and skip matching them during replay.
//
// Pattern is taken literally except for *, which matches any sequence of characters.
func (r *Recorder) RemoveResponseHeaders(patterns ...string) {
	r.proxy.RemoveResponseHeaders(patterns)
}

// ClearHeaders will replace the value of request and response headers that match
// any of the patterns with CLEARED, on both recording and replay.
// Use ClearHeaders when the header information is secret or may change from run to
// run, but you still want to verify that the headers are being sent and received.
//
// Pattern is taken literally except for *, which matches any sequence of characters.
func (r *Recorder) ClearHeaders(patterns ...string) {
	r.proxy.ClearHeaders(patterns)
}

// RemoveQueryParams will remove URL query parameters matching patterns from the log,
// and skip matching them during replay.
//
// Pattern is taken literally except for *, which matches any sequence of characters.
func (r *Recorder) RemoveQueryParams(patterns ...string) {
	r.proxy.RemoveQueryParams(patterns)
}

// ClearQueryParams will replace the value of URL query parametrs that match any of
// the patterns with CLEARED, on both recording and replay.
// Use ClearQueryParams when the parameter information is secret or may change from
// run to run, but you still want to verify that it are being sent.
//
// Pattern is taken literally except for *, which matches any sequence of characters.
func (r *Recorder) ClearQueryParams(patterns ...string) {
	r.proxy.ClearQueryParams(patterns)
}

// Client returns an http.Client to be used for recording.
func (r *Recorder) Client() *http.Client {
	return &http.Client{Transport: r.proxy.Transport()}
}

// ProxyURL will return the MITM proxy address
func (r *Recorder) ProxyURL() *url.URL {
	return r.proxy.URL
}

// Close closes the Recorder and saves the log file.
//
// Since Close writes a file, you should always check that
// it returns a non-nil error.
func (r *Recorder) Close() error {
	return r.proxy.Close()
}

// A Replayer replays previously recorded HTTP interactions.
type Replayer struct {
	proxy *proxy.Proxy
	port  int
	cert  string
	key   string
}

type replayerOption func(*Replayer)

// The custom CA cert filename to use for the MITM proxy to create certs on the
// fly
func ReplayerCert(fileName string) replayerOption {
	return func(r *Replayer) {
		r.cert = fileName
	}
}

// The key filename to the custom CA cert to use for the MITM proxy to create
// certs on the fly
func ReplayerKey(fileName string) replayerOption {
	return func(r *Replayer) {
		r.key = fileName
	}
}

// The port number for the MITM proxy
func ReplayerPort(port int) replayerOption {
	return func(r *Replayer) {
		r.port = port
	}
}

// NewReplayerWithOpts creates a replayer that reads from filename. The default
// Replayer MITM proxy can be customised with one or more replayerOption.
func NewReplayerWithOpts(filename string, opts ...replayerOption) (*Replayer, error) {
	r := &Replayer{port: 0, cert: "", key: ""}
	for _, opt := range opts {
		opt(r)
	}
	p, err := proxy.ForReplaying(filename, r.port, r.cert, r.key)
	if err != nil {
		return nil, err
	}
	r.proxy = p
	return r, nil
}

// NewReplayer creates a replayer that reads from filename.
func NewReplayer(filename string) (*Replayer, error) {
	p, err := proxy.ForReplaying(filename, 0, "", "")
	if err != nil {
		return nil, err
	}
	return &Replayer{proxy: p}, nil
}

// Client returns an HTTP client for replaying.
//
// The client does not need to be configured with credentials for authenticating to a
// server, since it never contacts a real backend.
func (r *Replayer) Client() *http.Client {
	return &http.Client{Transport: r.proxy.Transport()}
}

// ProxyURL will return the MITM proxy address
func (r *Replayer) ProxyURL() *url.URL {
	return r.proxy.URL
}

// Initial returns the initial state saved by the Recorder.
func (r *Replayer) Initial() []byte {
	return r.proxy.Initial
}

// IgnoreHeader will not use h when matching requests.
func (r *Replayer) IgnoreHeader(h string) {
	r.proxy.IgnoreHeader(h)
}

// Close closes the replayer.
func (r *Replayer) Close() error {
	return r.proxy.Close()
}

// DebugHeaders helps to determine whether a header should be ignored.
// When true, if requests have the same method, URL and body but differ
// in a header, the first mismatched header is logged.
func DebugHeaders() {
	proxy.DebugHeaders = true
}
