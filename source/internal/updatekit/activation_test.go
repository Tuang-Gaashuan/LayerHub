package updatekit

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestActivationFailureBeforeNewStartResumesOnlyOldProgram(t *testing.T) {
	for _, failure := range []string{"stop", "inspect", "changed", "backup", "switch"} {
		t.Run(failure, func(t *testing.T) {
			var events []string
			pointer := "old"
			calls := 0
			step := func(name string) error {
				events = append(events, name)
				if failure == name {
					return errors.New("fixture failure")
				}
				return nil
			}
			a := activation{before: []byte("before"), stop: func() error { return step("stop") }, start: func() error { calls++; return step("start") }, inspect: func() ([]byte, error) {
				if failure == "changed" {
					return []byte("different"), step("inspect")
				}
				return []byte("before"), step("inspect")
			}, backup: func() error { return step("backup") }, selectNew: func() error { pointer = "new"; return step("switch") }, selectOld: func() error { pointer = "old"; return step("restore-pointer") }}
			if e := activate(a); e == nil {
				t.Fatal("expected failure")
			}
			if pointer != "old" || calls != 1 {
				t.Fatal(pointer, calls, events)
			}
		})
	}
}
func TestNewProgramFailureNeverRestoresOldDataOrReader(t *testing.T) {
	data := filepath.Join(t.TempDir(), "database-fixture")
	os.WriteFile(data, []byte("old format"), 0o600)
	var events []string
	pointer := "old"
	appendStep := func(name string) error { events = append(events, name); return nil }
	a := activation{before: []byte("same"), stop: func() error { return appendStep("stop") }, inspect: func() ([]byte, error) { return []byte("same"), appendStep("inspect") }, backup: func() error { return appendStep("backup") }, selectNew: func() error { pointer = "new"; return appendStep("new-pointer") }, selectOld: func() error { pointer = "old"; return appendStep("old-pointer") }, start: func() error {
		appendStep("start-new")
		os.WriteFile(data, []byte("new format and business writes"), 0o600)
		return errors.New("new reader exited after a write")
	}}
	if e := activate(a); e == nil {
		t.Fatal("expected failure")
	}
	raw, _ := os.ReadFile(data)
	if pointer != "new" || string(raw) != "new format and business writes" || !reflect.DeepEqual(events, []string{"stop", "inspect", "backup", "new-pointer", "start-new"}) {
		t.Fatal(pointer, string(raw), events)
	}
}

func TestFailedPointerRecoveryNeverStartsUnknownReader(t *testing.T) {
	calls := 0
	a := activation{before: []byte("same"), stop: func() error { return nil }, start: func() error { calls++; return nil }, inspect: func() ([]byte, error) { return []byte("same"), nil }, backup: func() error { return nil }, selectNew: func() error { return errors.New("ambiguous switch") }, selectOld: func() error { return errors.New("cannot restore pointer") }}
	if e := activate(a); e == nil || calls != 0 {
		t.Fatal(e, calls)
	}
}
