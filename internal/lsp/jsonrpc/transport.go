package jsonrpc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// maxContentLength is the maximum allowed Content-Length (10 MiB).
// Prevents memory exhaustion from malicious or malformed headers.
const maxContentLength = 10 * 1024 * 1024

// Transport reads and writes JSON-RPC 2.0 messages with
// Content-Length framing (LSP base protocol).
type Transport struct {
	reader *bufio.Reader
	writer io.Writer
	wmu    sync.Mutex // serializes writes
}

// NewTransport creates a Transport over the given reader/writer.
func NewTransport(r io.Reader, w io.Writer) *Transport {
	return &Transport{
		reader: bufio.NewReaderSize(r, 64*1024),
		writer: w,
	}
}

// Read reads one message from the transport. Blocks until a complete
// message is available or an error occurs.
func (t *Transport) Read() (*Message, error) {
	contentLength := -1
	for {
		line, err := t.reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("read header: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break // end of headers
		}
		if after, ok := strings.CutPrefix(line, "Content-Length: "); ok {
			contentLength, err = strconv.Atoi(after)
			if err != nil {
				return nil, fmt.Errorf("invalid Content-Length: %w", err)
			}
		}
		// Ignore other headers (e.g., Content-Type).
	}
	if contentLength < 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}
	if contentLength > maxContentLength {
		return nil, fmt.Errorf("Content-Length %d exceeds maximum %d", contentLength, maxContentLength)
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(t.reader, body); err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var msg Message
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, &DecodeError{Cause: err}
	}
	return &msg, nil
}

// Write writes one message to the transport. Safe for concurrent use.
func (t *Transport) Write(msg Message) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("encode message: %w", err)
	}
	t.wmu.Lock()
	defer t.wmu.Unlock()
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := io.WriteString(t.writer, header); err != nil {
		return err
	}
	_, err = t.writer.Write(body)
	return err
}
