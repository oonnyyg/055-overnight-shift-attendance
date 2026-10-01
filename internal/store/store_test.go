package store

import (
	"path/filepath"
	"testing"
	"time"

	"example.com/overnight-shift-attendance/internal/model"
	"example.com/overnight-shift-attendance/internal/timex"
)

func mustTime(t *testing.T, s string) (tm time.Time) {
	t.Helper()
	v, err := timex.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RegisterEmployee(model.Employee{ID: "e1", Name: "Ann"}); err != nil {
		t.Fatal(err)
	}
	return s
}

func nightShift(t *testing.T, id string) model.Shift {
	t.Helper()
	return model.Shift{
		ID:    id,
		EmpID: "e1",
		Start: mustTime(t, "2026-10-01 22:00"),
		End:   mustTime(t, "2026-10-02 06:00"),
		Breaks: []timex.Interval{{
			Start: mustTime(t, "2026-10-02 02:00"),
			End:   mustTime(t, "2026-10-02 02:30"),
		}},
	}
}

func TestImportShiftsOvernightAndDuplicate(t *testing.T) {
	s := newTestStore(t)
	n, err := s.ImportShifts([]model.Shift{nightShift(t, "s1")})
	if err != nil || n != 1 {
		t.Fatalf("import = %d, %v", n, err)
	}
	n, err = s.ImportShifts([]model.Shift{nightShift(t, "s1")})
	if err != nil || n != 0 {
		t.Fatalf("duplicate import = %d, %v", n, err)
	}
	if len(s.Data().Shifts) != 1 {
		t.Fatalf("shifts = %d", len(s.Data().Shifts))
	}
}

func TestImportShiftsAtomicOnConflict(t *testing.T) {
	s := newTestStore(t)
	bad := nightShift(t, "s2")
	bad.Start = mustTime(t, "2026-10-01 20:00")
	bad.End = mustTime(t, "2026-10-01 20:00")
	if _, err := s.ImportShifts([]model.Shift{nightShift(t, "s1"), bad}); err == nil {
		t.Fatal("expected rejection of zero-length shift")
	}
	if len(s.Data().Shifts) != 0 {
		t.Fatal("failed batch must leave no records")
	}
}

func TestImportShiftsOverlap(t *testing.T) {
	s := newTestStore(t)
	other := nightShift(t, "s2")
	other.Start = mustTime(t, "2026-10-02 05:30")
	other.End = mustTime(t, "2026-10-02 08:00")
	other.Breaks = nil
	if _, err := s.ImportShifts([]model.Shift{nightShift(t, "s1"), other}); err == nil {
		t.Fatal("expected overlap rejection across midnight")
	}
}

func TestBreakOutsideShiftRejected(t *testing.T) {
	s := newTestStore(t)
	sh := nightShift(t, "s1")
	sh.Breaks = []timex.Interval{{
		Start: mustTime(t, "2026-10-01 21:00"),
		End:   mustTime(t, "2026-10-01 21:30"),
	}}
	if _, err := s.ImportShifts([]model.Shift{sh}); err == nil {
		t.Fatal("expected break-outside-shift rejection")
	}
}

func TestPunchValidationAndAtomicity(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.ImportShifts([]model.Shift{nightShift(t, "s1")}); err != nil {
		t.Fatal(err)
	}
	good := model.Punch{ID: "p1", EmpID: "e1", ShiftID: "s1", Kind: model.KindIn,
		At: mustTime(t, "2026-10-01 22:00")}
	badEmp := good
	badEmp.ID = "p2"
	badEmp.EmpID = "ghost"
	if _, err := s.ImportPunches([]model.Punch{good, badEmp}); err == nil {
		t.Fatal("expected unknown employee rejection")
	}
	if len(s.Data().Punches) != 0 {
		t.Fatal("failed punch batch must leave no records")
	}
	n, err := s.ImportPunches([]model.Punch{good, good})
	if err != nil || n != 1 {
		t.Fatalf("duplicate punch import = %d, %v", n, err)
	}
}

func TestPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RegisterEmployee(model.Employee{ID: "e1", Name: "Ann"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ImportShifts([]model.Shift{nightShift(t, "s1")}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ImportLeaves([]model.Leave{{
		ID: "l1", EmpID: "e1",
		Start: mustTime(t, "2026-10-02 03:00"),
		End:   mustTime(t, "2026-10-02 04:00"),
	}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(filepath.Join(dir)); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(s2.Data().Shifts) != 1 || len(s2.Data().Leaves) != 1 {
		t.Fatal("data did not survive reload")
	}
}
