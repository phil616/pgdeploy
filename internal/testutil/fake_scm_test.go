package testutil

import (
	"testing"

	"github.com/pgdeploy/pgdeploy/internal/winservice"
)

func TestFakeServiceManagerLifecycle(t *testing.T) {
	m := NewFakeServiceManager()

	spec := winservice.ServiceSpec{Name: "pg", StartType: winservice.StartAutomatic}
	if err := m.Create(spec); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := m.Create(spec); err == nil {
		t.Fatal("duplicate Create should fail")
	}

	if ok, _ := m.Exists("pg"); !ok {
		t.Fatal("Exists(pg) = false")
	}
	if ok, _ := m.Exists("missing"); ok {
		t.Fatal("Exists(missing) = true")
	}

	if err := m.Start("pg"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	st, err := m.Query("pg")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if st.State != winservice.StateRunning {
		t.Errorf("state = %q, want %q", st.State, winservice.StateRunning)
	}

	if err := m.Stop("pg"); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if err := m.SetStartType("pg", winservice.StartManual); err != nil {
		t.Fatalf("SetStartType: %v", err)
	}
	cfg, err := m.Config("pg")
	if err != nil {
		t.Fatalf("Config: %v", err)
	}
	if cfg.StartType != winservice.StartManual {
		t.Errorf("StartType = %d, want %d", cfg.StartType, winservice.StartManual)
	}

	// Delete is idempotent.
	if err := m.Delete("pg"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := m.Delete("pg"); err != nil {
		t.Fatal("second Delete should be idempotent")
	}
	if err := m.Delete("never-existed"); err != nil {
		t.Fatal("Delete of a missing service should be idempotent")
	}
}
