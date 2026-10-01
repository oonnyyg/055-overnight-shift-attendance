package main

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type cliResp struct {
	OK     bool            `json:"ok"`
	Error  string          `json:"error"`
	Added  int             `json:"added"`
	Shifts json.RawMessage `json:"shifts"`
}

type shiftReport struct {
	ShiftID     string `json:"shift_id"`
	RequiredMin int    `json:"required_minutes"`
	WorkMin     int    `json:"work_minutes"`
	AbsentMin   int    `json:"absent_minutes"`
	MissingCard bool   `json:"missing_card"`
}

func TestCLIPipeline(t *testing.T) {
	dir := t.TempDir()

	lines := []string{
		`{"op":"register_employee","payload":{"id":"e1","name":"Ann"}}`,
		`{"op":"import_shifts","payload":[{"id":"s1","employee_id":"e1","start":"2026-10-01 09:00","end":"2026-10-01 18:00","breaks":[{"start":"2026-10-01 12:00","end":"2026-10-01 13:00"}]},{"id":"s2","employee_id":"e1","start":"2026-10-02 09:00","end":"2026-10-02 12:00"}]}`,
		`{"op":"import_leaves","payload":[{"id":"l1","employee_id":"e1","start":"2026-10-02 08:00","end":"2026-10-02 13:00"}]}`,
		`{"op":"import_punches","payload":[{"id":"p2","employee_id":"e1","shift_id":"s1","kind":"out","at":"2026-10-01 18:00"},{"id":"p1","employee_id":"e1","shift_id":"s1","kind":"in","at":"2026-10-01 09:00"},{"id":"p3","employee_id":"e1","shift_id":"s1","kind":"in","at":"2026-10-01 20:00"}]}`,
		`{"op":"report","payload":{"employee_id":"e1"}}`,
	}

	cmd := exec.Command("go", "run", ".", "-dir", filepath.Join(dir, "data"))
	cmd.Stdin = strings.NewReader(strings.Join(lines, "\n") + "\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run failed: %v\n%s", err, out)
	}
	got := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(got) != 5 {
		t.Fatalf("want 5 responses, got %d: %s", len(got), out)
	}

	for i, line := range got[:4] {
		var r cliResp
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("line %d bad json: %v", i, err)
		}
		if !r.OK {
			t.Fatalf("line %d error: %s", i, r.Error)
		}
	}

	var final struct {
		Shifts []shiftReport `json:"shifts"`
	}
	var rawFinal cliResp
	if err := json.Unmarshal([]byte(got[4]), &rawFinal); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(rawFinal.Shifts, &final.Shifts); err != nil {
		t.Fatal(err)
	}
	if len(final.Shifts) != 2 {
		t.Fatalf("reports = %d", len(final.Shifts))
	}
	r1 := final.Shifts[0]
	if r1.ShiftID != "s1" || !r1.MissingCard || r1.WorkMin != 0 || r1.AbsentMin != 8*60 {
		t.Fatalf("off-hours extra punch should make missing card: %+v", r1)
	}
	r2 := final.Shifts[1]
	if r2.ShiftID != "s2" || r2.RequiredMin != 0 || r2.AbsentMin != 0 {
		t.Fatalf("whole-shift leave still listed but zero: %+v", r2)
	}
	var summary struct {
		Employees []struct {
			EmployeeID    string `json:"employee_id"`
			MissingShifts int    `json:"missing_card_shifts"`
		} `json:"employees"`
	}
	if err := json.Unmarshal([]byte(got[4]), &summary); err != nil {
		t.Fatal(err)
	}
	if len(summary.Employees) != 1 || summary.Employees[0].MissingShifts != 1 {
		t.Fatalf("employee summary wrong: %+v", summary.Employees)
	}

	cmd = exec.Command("go", "run", ".", "-dir", filepath.Join(dir, "data"))
	cmd.Stdin = strings.NewReader(`{"op":"report","payload":{}}` + "\n")
	out2, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out2), `"required_minutes":0`) {
		t.Fatalf("reopened result mismatch: %s", out2)
	}
}

func TestCLIBadLineDoesNotStopStream(t *testing.T) {
	dir := t.TempDir()
	input := strings.Join([]string{
		`{"op":"nope"}`,
		`{"op":"register_employee","payload":{"id":"e1","name":"Ann"}}`,
		"not json",
	}, "\n") + "\n"
	cmd := exec.Command("go", "run", ".", "-dir", filepath.Join(dir, "data"))
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 responses got %d: %s", len(lines), out)
	}
	if !strings.Contains(lines[0], `"ok":false`) ||
		!strings.Contains(lines[1], `"ok":true`) ||
		!strings.Contains(lines[2], `"ok":false`) {
		t.Fatalf("unexpected responses: %s", out)
	}
}
