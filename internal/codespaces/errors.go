package codespaces

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// httpError wraps a non-2xx response so FormatError can pretty-print the
// google.rpc.Status JSON body that the codespaces grpc-gateway returns
// (BadRequest field violations, ErrorInfo, etc.).
type httpError struct {
	Method     string
	URL        string
	StatusCode int
	Body       []byte
}

func (e *httpError) Error() string {
	msg := strings.TrimSpace(string(e.Body))
	if msg == "" {
		msg = http.StatusText(e.StatusCode)
	}
	return fmt.Sprintf("%s %s: %d %s", e.Method, e.URL, e.StatusCode, msg)
}

// google.rpc.Status as the gRPC-gateway serializes it. Details is a list of
// "any"-typed messages; we sniff @type and re-decode the known shapes.

type googleRPCStatus struct {
	Code    int               `json:"code,omitempty"`
	Message string            `json:"message,omitempty"`
	Details []json.RawMessage `json:"details,omitempty"`
}

type googleRPCDetailHeader struct {
	Type string `json:"@type"`
}

type googleRPCFieldViolation struct {
	Field       string `json:"field,omitempty"`
	Description string `json:"description,omitempty"`
}

type googleRPCBadRequest struct {
	FieldViolations []googleRPCFieldViolation `json:"fieldViolations,omitempty"`
}

type googleRPCErrorInfo struct {
	Reason string `json:"reason,omitempty"`
	Domain string `json:"domain,omitempty"`
}

// FormatError returns a human-readable string for any error, expanding
// google.rpc.Status field violations and ErrorInfo when the underlying
// transport error is one we produced. Returns "" for nil err.
func FormatError(err error) string {
	if err == nil {
		return ""
	}
	var he *httpError
	if !errors.As(err, &he) {
		return err.Error()
	}

	var st googleRPCStatus
	if jsonErr := json.Unmarshal(he.Body, &st); jsonErr != nil {
		return he.Error()
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s %s: HTTP %d", he.Method, he.URL, he.StatusCode)
	if st.Message != "" {
		fmt.Fprintf(&b, ": %s", st.Message)
	}
	for _, raw := range st.Details {
		var hdr googleRPCDetailHeader
		if err := json.Unmarshal(raw, &hdr); err != nil {
			continue
		}
		switch hdr.Type {
		case "type.googleapis.com/google.rpc.BadRequest":
			var br googleRPCBadRequest
			if uerr := json.Unmarshal(raw, &br); uerr == nil {
				for _, fv := range br.FieldViolations {
					fmt.Fprintf(&b, "\n  - %s: %s", fv.Field, fv.Description)
				}
			}
		case "type.googleapis.com/google.rpc.ErrorInfo":
			var ei googleRPCErrorInfo
			if uerr := json.Unmarshal(raw, &ei); uerr == nil {
				fmt.Fprintf(&b, "\n  reason=%s domain=%s", ei.Reason, ei.Domain)
			}
		}
	}
	return b.String()
}
