// Package jsonrpc implements a minimal JSON-RPC 2.0 transport for the
// Language Server Protocol. It provides Content-Length framing over
// stdin/stdout and message type discrimination.
package jsonrpc

import "encoding/json"

// Message is a JSON-RPC 2.0 message (request, response, or notification).
type Message struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *ResponseError   `json:"error,omitempty"`
}

// ResponseError is a JSON-RPC 2.0 error object.
type ResponseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// DecodeError indicates a JSON decode failure on a received message.
// The transport can continue reading after this error.
type DecodeError struct {
	Cause error
}

func (e *DecodeError) Error() string { return "decode message: " + e.Cause.Error() }
func (e *DecodeError) Unwrap() error { return e.Cause }

// Standard JSON-RPC and LSP error codes.
const (
	CodeParseError       = -32700
	CodeInvalidRequest   = -32600
	CodeMethodNotFound   = -32601
	CodeInvalidParams    = -32602
	CodeInternalError    = -32603
	CodeRequestCancelled = -32800
)

// IsRequest returns true if the message is a request (has method and id).
func (m *Message) IsRequest() bool { return m.Method != "" && m.ID != nil }

// IsNotification returns true if the message is a notification (method, no id).
func (m *Message) IsNotification() bool { return m.Method != "" && m.ID == nil }

// IsResponse returns true if the message is a response (id, no method).
func (m *Message) IsResponse() bool { return m.Method == "" && m.ID != nil }

// NewResponse creates a success response message.
func NewResponse(id *json.RawMessage, result any) (Message, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return Message{}, err
	}
	return Message{
		JSONRPC: "2.0",
		ID:      id,
		Result:  data,
	}, nil
}

// NewError creates an error response message.
func NewError(id *json.RawMessage, code int, message string) Message {
	return Message{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &ResponseError{Code: code, Message: message},
	}
}

// NewNotification creates a notification message (no id, no response expected).
func NewNotification(method string, params any) (Message, error) {
	data, err := json.Marshal(params)
	if err != nil {
		return Message{}, err
	}
	return Message{
		JSONRPC: "2.0",
		Method:  method,
		Params:  data,
	}, nil
}
