package codespaces

import (
	"fmt"
	"strings"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/status"
)

// FormatError returns a human-readable string for any error, expanding
// gRPC BadRequest field violations when present. Returns "" for a nil error.
func FormatError(err error) string {
	if err == nil {
		return ""
	}

	st, ok := status.FromError(err)
	if !ok {
		return err.Error()
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s", st.Code(), st.Message())
	for _, d := range st.Details() {
		switch v := d.(type) {
		case *errdetails.BadRequest:
			for _, fv := range v.GetFieldViolations() {
				fmt.Fprintf(&b, "\n  - %s: %s", fv.GetField(), fv.GetDescription())
			}
		case *errdetails.ErrorInfo:
			fmt.Fprintf(&b, "\n  reason=%s domain=%s", v.GetReason(), v.GetDomain())
		}
	}
	return b.String()
}
