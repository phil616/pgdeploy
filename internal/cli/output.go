package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/pgdeploy/pgdeploy/internal/diagnostics"
	"github.com/pgdeploy/pgdeploy/internal/install"
)

// printReport renders an operation report to w. In JSON mode it emits a single
// structured object (the Report never contains secrets).
func printReport(w io.Writer, r install.Report, jsonOut bool) {
	if jsonOut {
		b, err := json.Marshal(r)
		if err != nil {
			b = []byte(`{"status":"error"}`)
		}
		fmt.Fprintln(w, string(b))
		return
	}
	emit := func(label, value string) {
		if value != "" {
			fmt.Fprintf(w, "%s: %s\n", label, value)
		}
	}
	emit("status", r.Status)
	emit("code", r.Code)
	emit("root", r.Root)
	emit("service", r.ServiceName)
	if r.Port != 0 {
		fmt.Fprintf(w, "port: %d\n", r.Port)
	}
	emit("data_directory", r.DataDir)
	emit("postgres_version", r.PostgresVersion)
	emit("connection_url", r.ConnectionURL)
	if r.CredentialsHint != "" {
		fmt.Fprintf(w, "credentials: %s\n", r.CredentialsHint)
	}
	for _, warn := range r.Warnings {
		fmt.Fprintf(w, "warning: %s\n", warn)
	}
	for _, c := range r.Checks {
		fmt.Fprintf(w, "[%s] %s", c.Status, c.Name)
		if c.Detail != "" {
			fmt.Fprintf(w, ": %s", c.Detail)
		}
		fmt.Fprintln(w)
	}
}

// printError renders an error to w (stderr) in human or JSON form. It never
// echoes secrets; callers must already have redacted secret-bearing strings.
func printError(w io.Writer, err error, jsonOut bool) {
	if err == nil {
		return
	}
	if jsonOut {
		obj := map[string]string{"code": diagnostics.CodeInternal, "message": err.Error()}
		if de, ok := err.(*diagnostics.Error); ok {
			obj["code"] = de.Code
			obj["message"] = de.Message
			if de.Detail != "" {
				obj["detail"] = de.Detail
			}
			if de.Recoverable {
				obj["recoverable"] = "true"
			}
		}
		if b, e := json.Marshal(obj); e == nil {
			fmt.Fprintln(w, string(b))
		}
		return
	}
	if de, ok := err.(*diagnostics.Error); ok {
		fmt.Fprintf(w, "error [%s]: %s\n", de.Code, de.Message)
		if de.Detail != "" && de.Detail != de.Message {
			fmt.Fprintf(w, "  detail: %s\n", de.Detail)
		}
		if de.Recoverable {
			fmt.Fprintln(w, "  this error is recoverable; re-run the command or use `repair`/`doctor`")
		}
		return
	}
	fmt.Fprintf(w, "error: %v\n", err)
}
