package client

import (
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// Response represents a generic API response containing the raw body bytes.
type Response struct {
	body []byte
}

// Body returns the raw response body as a byte slice.
func (r *Response) Body() []byte {
	return r.body
}

// ProtoUnmarshal unmarshals the response body into the provided proto.Message.
func (r *Response) ProtoUnmarshal(out proto.Message) error {
	return protojson.Unmarshal(r.body, out)
}

// NewResponse creates a new Response with the given body bytes.
func NewResponse(body []byte) *Response {
	return &Response{
		body: body,
	}
}
