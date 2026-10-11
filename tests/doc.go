// Package tests holds end-to-end integration tests that exercise a real
// install against a real PostgreSQL ZIP and the real SCM. They are gated
// behind the `integration` build tag and PGDEPLOY_INTEGRATION=1 so they never
// run during a normal `go test ./...`.
package tests
