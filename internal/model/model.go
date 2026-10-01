package model

import (
	"time"

	"example.com/overnight-shift-attendance/internal/timex"
)

// KindIn / KindOut mark a punch as entering or leaving.
const (
	KindIn  = "in"
	KindOut = "out"
)

// Employee is a registered worker.
type Employee struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Shift is a scheduled working interval with optional breaks.
type Shift struct {
	ID     string           `json:"id"`
	EmpID  string           `json:"employee_id"`
	Start  time.Time        `json:"start"`
	End    time.Time        `json:"end"`
	Breaks []timex.Interval `json:"breaks"`
}

// Leave is an approved leave interval for an employee.
type Leave struct {
	ID    string    `json:"id"`
	EmpID string    `json:"employee_id"`
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Punch is one enter/leave clock event.
type Punch struct {
	ID      string    `json:"id"`
	EmpID   string    `json:"employee_id"`
	ShiftID string    `json:"shift_id"`
	Kind    string    `json:"kind"`
	At      time.Time `json:"at"`
}

// Store is the whole on-disk data set.
type Store struct {
	Employees map[string]Employee `json:"employees"`
	Shifts    map[string]Shift    `json:"shifts"`
	Leaves    map[string]Leave    `json:"leaves"`
	Punches   map[string]Punch    `json:"punches"`
}

// NewStore returns an empty initialized store.
func NewStore() *Store {
	return &Store{
		Employees: map[string]Employee{},
		Shifts:    map[string]Shift{},
		Leaves:    map[string]Leave{},
		Punches:   map[string]Punch{},
	}
}
