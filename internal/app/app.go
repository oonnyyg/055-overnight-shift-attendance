// Package app wires persistence, validation and the reporting engine into
// request handlers used by the command line entry point.
package app

import (
	"sort"

	"example.com/overnight-shift-attendance/internal/engine"
	"example.com/overnight-shift-attendance/internal/model"
	"example.com/overnight-shift-attendance/internal/store"
	"example.com/overnight-shift-attendance/internal/timex"
)

// App is the application service bound to one data directory.
type App struct {
	dir   string
	store *store.Store
}

// New opens (or initializes) the data directory.
func New(dir string) (*App, error) {
	s, err := store.Open(dir)
	if err != nil {
		return nil, err
	}
	return &App{dir: dir, store: s}, nil
}

// EmployeeInput is the register-employee request body.
type EmployeeInput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type intervalInput struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// ShiftInput is one item of import-shifts.
type ShiftInput struct {
	ID     string          `json:"id"`
	EmpID  string          `json:"employee_id"`
	Start  string          `json:"start"`
	End    string          `json:"end"`
	Breaks []intervalInput `json:"breaks"`
}

// LeaveInput is one item of import-leaves.
type LeaveInput struct {
	ID    string `json:"id"`
	EmpID string `json:"employee_id"`
	Start string `json:"start"`
	End   string `json:"end"`
}

// PunchInput is one item of import-punches.
type PunchInput struct {
	ID      string `json:"id"`
	EmpID   string `json:"employee_id"`
	ShiftID string `json:"shift_id"`
	Kind    string `json:"kind"`
	At      string `json:"at"`
}

// ReportInput filters reports by employee and inclusive date range.
type ReportInput struct {
	EmpID     string `json:"employee_id"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

// RegisterEmployee registers or idempotently re-registers one employee.
func (a *App) RegisterEmployee(in EmployeeInput) error {
	if err := a.store.RegisterEmployee(model.Employee{ID: in.ID, Name: in.Name}); err != nil {
		return err
	}
	return a.store.Save(a.dir)
}

// ImportShifts atomically imports shifts.
func (a *App) ImportShifts(in []ShiftInput) (int, error) {
	shifts, err := convertShifts(in)
	if err != nil {
		return 0, err
	}
	added, err := a.store.ImportShifts(shifts)
	if err != nil {
		return 0, err
	}
	return added, a.store.Save(a.dir)
}

// ImportLeaves atomically imports leaves.
func (a *App) ImportLeaves(in []LeaveInput) (int, error) {
	leaves := make([]model.Leave, len(in))
	for i, item := range in {
		st, err := timex.Parse(item.Start)
		if err != nil {
			return 0, parseErr("leaves", i, "start", item.Start)
		}
		en, err := timex.Parse(item.End)
		if err != nil {
			return 0, parseErr("leaves", i, "end", item.End)
		}
		leaves[i] = model.Leave{ID: item.ID, EmpID: item.EmpID, Start: st, End: en}
	}
	added, err := a.store.ImportLeaves(leaves)
	if err != nil {
		return 0, err
	}
	return added, a.store.Save(a.dir)
}

// ImportPunches atomically imports punches.
func (a *App) ImportPunches(in []PunchInput) (int, error) {
	punches := make([]model.Punch, len(in))
	for i, item := range in {
		ts, err := timex.Parse(item.At)
		if err != nil {
			return 0, parseErr("punches", i, "at", item.At)
		}
		punches[i] = model.Punch{
			ID: item.ID, EmpID: item.EmpID, ShiftID: item.ShiftID,
			Kind: item.Kind, At: ts,
		}
	}
	added, err := a.store.ImportPunches(punches)
	if err != nil {
		return 0, err
	}
	return added, a.store.Save(a.dir)
}

func convertShifts(in []ShiftInput) ([]model.Shift, error) {
	shifts := make([]model.Shift, len(in))
	for i, item := range in {
		st, err := timex.Parse(item.Start)
		if err != nil {
			return nil, parseErr("shifts", i, "start", item.Start)
		}
		en, err := timex.Parse(item.End)
		if err != nil {
			return nil, parseErr("shifts", i, "end", item.End)
		}
		sh := model.Shift{ID: item.ID, EmpID: item.EmpID, Start: st, End: en}
		for _, br := range item.Breaks {
			bs, err := timex.Parse(br.Start)
			if err != nil {
				return nil, parseErr("shifts", i, "break start", br.Start)
			}
			be, err := timex.Parse(br.End)
			if err != nil {
				return nil, parseErr("shifts", i, "break end", br.End)
			}
			sh.Breaks = append(sh.Breaks, timex.Interval{Start: bs, End: be})
		}
		shifts[i] = sh
	}
	return shifts, nil
}

func parseErr(batch string, idx int, field, value string) error {
	return &ParseError{Batch: batch, Index: idx, Field: field, Value: value}
}

// ParseError marks a malformed time string in an import batch.
type ParseError struct {
	Batch string
	Index int
	Field string
	Value string
}

func (e *ParseError) Error() string {
	return e.Batch + "[" + itoa(e.Index) + "] " + e.Field + ": invalid time " + quote(e.Value)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func quote(s string) string { return "\"" + s + "\"" }

// ReportResponse bundles per-shift detail with per-employee totals.
type ReportResponse struct {
	Shifts   []engine.ShiftReport     `json:"shifts"`
	Employee []engine.EmployeeSummary `json:"employees"`
}

// Report builds reports filtered by employee and inclusive dates.
// An empty employee id means all employees.
func (a *App) Report(in ReportInput) (*ReportResponse, error) {
	start, end, err := dateRange(in.StartDate, in.EndDate)
	if err != nil {
		return nil, err
	}
	d := a.store.Data()

	shifts := make([]model.Shift, 0, len(d.Shifts))
	for _, sh := range d.Shifts {
		if in.EmpID != "" && sh.EmpID != in.EmpID {
			continue
		}
		day := timex.Date(sh.Start)
		if start != "" && day < start {
			continue
		}
		if end != "" && day > end {
			continue
		}
		shifts = append(shifts, sh)
	}
	sort.Slice(shifts, func(i, j int) bool {
		if shifts[i].EmpID != shifts[j].EmpID {
			return shifts[i].EmpID < shifts[j].EmpID
		}
		if shifts[i].Start.Equal(shifts[j].Start) {
			return shifts[i].ID < shifts[j].ID
		}
		return shifts[i].Start.Before(shifts[j].Start)
	})

	leaves := make([]model.Leave, 0, len(d.Leaves))
	for _, lv := range d.Leaves {
		leaves = append(leaves, lv)
	}

	resp := &ReportResponse{Shifts: []engine.ShiftReport{}}
	for _, sh := range shifts {
		var punches []model.Punch
		for _, p := range d.Punches {
			if p.ShiftID == sh.ID {
				punches = append(punches, p)
			}
		}
		resp.Shifts = append(resp.Shifts, engine.Build(sh, leaves, punches))
	}
	resp.Employee = engine.Summarize(resp.Shifts)
	return resp, nil
}

func dateRange(start, end string) (string, string, error) {
	if start != "" {
		if _, err := timex.Parse(start + " 00:00"); err != nil {
			return "", "", errInvalidDate(start)
		}
	}
	if end != "" {
		if _, err := timex.Parse(end + " 00:00"); err != nil {
			return "", "", errInvalidDate(end)
		}
	}
	if start != "" && end != "" && start > end {
		return "", "", errRangeReversed(start, end)
	}
	return start, end, nil
}

type simpleError string

func (e simpleError) Error() string { return string(e) }

func errInvalidDate(v string) error {
	return simpleError("invalid date " + quote(v) + ", want YYYY-MM-DD")
}

func errRangeReversed(a, b string) error {
	return simpleError("start_date " + a + " is after end_date " + b)
}
