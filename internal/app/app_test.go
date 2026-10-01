package app

import (
	"testing"
)

func newApp(t *testing.T) *App {
	t.Helper()
	a, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestEndToEndOvernightAndSummary(t *testing.T) {
	a := newApp(t)
	if err := a.RegisterEmployee(EmployeeInput{ID: "e1", Name: "Ann"}); err != nil {
		t.Fatal(err)
	}
	if err := a.RegisterEmployee(EmployeeInput{ID: "e2", Name: "Bob"}); err != nil {
		t.Fatal(err)
	}

	added, err := a.ImportShifts([]ShiftInput{
		{ID: "s1", EmpID: "e1", Start: "2026-10-01 22:00", End: "2026-10-02 06:00",
			Breaks: []intervalInput{{Start: "2026-10-02 02:00", End: "2026-10-02 02:30"}}},
		{ID: "s2", EmpID: "e2", Start: "2026-10-01 09:00", End: "2026-10-01 18:00"},
	})
	if err != nil || added != 2 {
		t.Fatalf("shifts added=%d err=%v", added, err)
	}

	punches := []PunchInput{
		{ID: "p2", EmpID: "e1", ShiftID: "s1", Kind: "out", At: "2026-10-02 06:00"},
		{ID: "p1", EmpID: "e1", ShiftID: "s1", Kind: "in", At: "2026-10-01 22:00"},
	}
	if added, err := a.ImportPunches(punches); err != nil || added != 2 {
		t.Fatalf("punches added=%d err=%v", added, err)
	}

	resp, err := a.Report(ReportInput{StartDate: "2026-10-01", EndDate: "2026-10-01"})
	if err != nil {
		t.Fatal(err)
	}
	reports := resp.Shifts
	if len(reports) != 2 {
		t.Fatalf("both shifts must attribute to start date, got %d", len(reports))
	}
	var night, bob int
	for _, r := range reports {
		if r.ShiftID == "s1" {
			night = r.WorkMin
			if r.RequiredMin != 8*60-30 || r.WorkMin != r.RequiredMin {
				t.Fatalf("night shift required=%d work=%d", r.RequiredMin, r.WorkMin)
			}
		}
		if r.ShiftID == "s2" {
			bob = r.AbsentMin
			if r.AbsentMin != 9*60 {
				t.Fatalf("no-punch shift absent=%d", r.AbsentMin)
			}
		}
	}
	if night == 0 || bob == 0 {
		t.Fatal("summary values unexpected")
	}

	onlyResp, err := a.Report(ReportInput{EmpID: "e1"})
	onlyAnn := onlyResp.Shifts
	if err != nil || len(onlyAnn) != 1 || onlyAnn[0].ShiftID != "s1" {
		t.Fatalf("employee filter failed: %+v %v", onlyResp, err)
	}
	if len(resp.Employee) != 2 {
		t.Fatalf("expected 2 employee summaries, got %d", len(resp.Employee))
	}
	if resp.Employee[0].AnomalyRecords != 0 || resp.Employee[1].ShiftCount != 1 {
		t.Fatalf("summary wrong: %+v", resp.Employee)
	}
}

func TestReopenDeterministic(t *testing.T) {
	dir := t.TempDir()
	a, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.RegisterEmployee(EmployeeInput{ID: "e1", Name: "Ann"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ImportShifts([]ShiftInput{{
		ID: "s1", EmpID: "e1", Start: "2026-10-01 09:00", End: "2026-10-01 18:00",
	}}); err != nil {
		t.Fatal(err)
	}
	b, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	resp2, err := b.Report(ReportInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp2.Shifts) != 1 {
		t.Fatalf("reopen lost data: %d", len(resp2.Shifts))
	}
}

func TestRejections(t *testing.T) {
	a := newApp(t)
	if err := a.RegisterEmployee(EmployeeInput{ID: "e1", Name: "Ann"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ImportShifts([]ShiftInput{{
		ID: "s1", EmpID: "e1", Start: "2026-10-01 09:00", End: "2026-10-01 08:00",
	}}); err == nil {
		t.Fatal("reversed range must be rejected")
	}
	if _, err := a.ImportShifts([]ShiftInput{{
		ID: "s1", EmpID: "e1", Start: "2026-10-01 09:00", End: "bad",
	}}); err == nil {
		t.Fatal("illegal time must be rejected")
	}
	if _, err := a.ImportShifts([]ShiftInput{{
		ID: "s1", EmpID: "ghost", Start: "2026-10-01 09:00", End: "2026-10-01 18:00",
	}}); err == nil {
		t.Fatal("unknown employee must be rejected")
	}
	if _, err := a.ImportPunches([]PunchInput{{
		ID: "p1", EmpID: "e1", ShiftID: "ghost", Kind: "in", At: "2026-10-01 09:00",
	}}); err == nil {
		t.Fatal("unknown shift must be rejected")
	}
}

func TestBatchAtomicityAcrossApp(t *testing.T) {
	a := newApp(t)
	if err := a.RegisterEmployee(EmployeeInput{ID: "e1", Name: "Ann"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ImportShifts([]ShiftInput{
		{ID: "s1", EmpID: "e1", Start: "2026-10-01 09:00", End: "2026-10-01 18:00"},
	}); err != nil {
		t.Fatal(err)
	}
	_, err := a.ImportPunches([]PunchInput{
		{ID: "p1", EmpID: "e1", ShiftID: "s1", Kind: "in", At: "2026-10-01 09:00"},
		{ID: "p2", EmpID: "e1", ShiftID: "s1", Kind: "sideways", At: "2026-10-01 10:00"},
	})
	if err == nil {
		t.Fatal("bad punch kind must reject the whole batch")
	}
	resp3, _ := a.Report(ReportInput{})
	if len(resp3.Shifts[0].Punches) != 0 {
		t.Fatal("failed batch must not leave partial punches")
	}
}
