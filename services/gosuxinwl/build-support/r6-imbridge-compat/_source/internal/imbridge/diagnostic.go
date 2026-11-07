package imbridge

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strings"

	mysql "github.com/go-sql-driver/mysql"
)

type operationFault struct {
	stage string
	cause error
}

func (e *operationFault) Error() string { return e.stage + ": " + Diagnostic(e.cause) }
func (e *operationFault) Unwrap() error { return e.cause }
func fault(stage string, err error) error {
	if err == nil {
		return nil
	}
	return &operationFault{stage, err}
}

// Diagnostic emits fixed operation names and error classes/numeric MySQL
// codes. It never emits an underlying query, DSN, password or employee value.
func Diagnostic(err error) string {
	if err == nil {
		return "class=none"
	}
	stage := "unspecified"
	var op *operationFault
	if errors.As(err, &op) {
		stage = op.stage
	}
	class := "unknown"
	detail := ""
	var db *mysql.MySQLError
	var network net.Error
	leaf := err
	for errors.Unwrap(leaf) != nil {
		leaf = errors.Unwrap(leaf)
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		class = "timeout"
	case errors.Is(err, context.Canceled):
		class = "canceled"
	case errors.Is(err, sql.ErrNoRows):
		class = "missing_row"
	case errors.As(err, &db):
		class = "mysql"
		detail = fmt.Sprintf(" dbCode=%d", db.Number)
	case errors.As(err, &network):
		class = "network"
	case strings.Contains(leaf.Error(), "converting NULL"):
		class = "null_projection"
	case strings.Contains(leaf.Error(), "Scan error"):
		class = "scan_type"
	case strings.Contains(leaf.Error(), "missing IM permission"):
		class = "missing_im_permission"
	}
	return "stage=" + stage + " class=" + class + detail
}
