package codespaces

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	rpcstatus "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/encoding/protojson"
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

	var st rpcstatus.Status
	if jsonErr := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(he.Body, &st); jsonErr != nil {
		return he.Error()
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s %s: HTTP %d", he.Method, he.URL, he.StatusCode)
	if msg := st.GetMessage(); msg != "" {
		fmt.Fprintf(&b, ": %s", msg)
	}
	for _, d := range st.GetDetails() {
		switch d.GetTypeUrl() {
		case "type.googleapis.com/google.rpc.BadRequest":
			var br errdetails.BadRequest
			if uerr := d.UnmarshalTo(&br); uerr == nil {
				for _, fv := range br.GetFieldViolations() {
					fmt.Fprintf(&b, "\n  - %s: %s", fv.GetField(), fv.GetDescription())
				}
			}
		case "type.googleapis.com/google.rpc.ErrorInfo":
			var ei errdetails.ErrorInfo
			if uerr := d.UnmarshalTo(&ei); uerr == nil {
				fmt.Fprintf(&b, "\n  reason=%s domain=%s", ei.GetReason(), ei.GetDomain())
			}
		}
	}
	return b.String()
}
