//go:build windows

// Command pgdeploy installs and manages PostgreSQL 18 on Windows.
package main

import (
	"os"

	"github.com/pgdeploy/pgdeploy/internal/cli"
	"github.com/pgdeploy/pgdeploy/internal/install"
	"github.com/pgdeploy/pgdeploy/internal/winservice"
)

func main() {
	app := &cli.App{
		Installer: install.New(),
		Elevator:  winservice.NewElevator(),
		Stdin:     os.Stdin,
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
	}
	os.Exit(app.Run(os.Args[1:]))
}
