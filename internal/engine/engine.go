// Package engine computes traceable attendance results from stored data.
package engine

import (
	"sort"

	"example.com/overnight-shift-attendance/internal/model"
	"example.com/overnight-shift-attendance/internal/timex"
)

// Anomaly describes one punch-related anomaly on a shift.
type Anomaly struct {
	Reason string         `json:"reason"`
	Punch  string         `json:"punch_id,omitempty"`
	Kind   string         `json:"kind,omitempty"`
	At     timex.JSONTime `json:"at,omitempty"`
}

// IntervalJSON is an interval rendered in canonical strings.
type IntervalJSON struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// PunchJSON is one raw punch in report order.
type PunchJSON struct {
	ID   string         `json:"id"`
	Kind string         `json:"kind"`
	At   timex.JSONTime `json:"at"`
}

// ShiftReport is the full per-shift result.
type ShiftReport struct {
	ShiftID     string         `json:"shift_id"`
	EmployeeID  string         `json:"employee_id"`
	Date        string         `json:"date"`
	ShiftStart  timex.JSONTime `json:"shift_start"`
	ShiftEnd    timex.JSONTime `json:"shift_end"`
	Breaks      []IntervalJSON `json:"breaks"`
	Leaves      []IntervalJSON `json:"leave_deductions"`
	Required    []IntervalJSON `json:"required_intervals"`
	RequiredMin int            `json:"required_minutes"`
	Punches     []PunchJSON    `json:"punches"`
	Presence    []IntervalJSON `json:"presence_intervals"`
	Anomalies   []Anomaly      `json:"anomalies"`
	MissingCard bool           `json:"missing_card"`
	WorkMin     int            `json:"work_minutes"`
	LateMin     int            `json:"late_minutes"`
	EarlyMin    int            `json:"early_minutes"`
	AbsentMin   int            `json:"absent_minutes"`
}

// PairPunches groups punches per shift and pairs them into presence intervals.
// Sorted by time then punch id; any pairing problem marks the whole shift as
// missing-card and returns the offending punch ids in anomalies.
func PairPunches(punches []model.Punch) (presence []timex.Interval, anomalies []Anomaly, missing bool) {
	if len(punches) == 0 {
		return nil, nil, false
	}
	sorted := make([]model.Punch, len(punches))
	copy(sorted, punches)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].At.Equal(sorted[j].At) {
			return sorted[i].ID < sorted[j].ID
		}
		return sorted[i].At.Before(sorted[j].At)
	})

	var open *model.Punch
	for i := range sorted {
		p := &sorted[i]
		switch p.Kind {
		case model.KindIn:
			if open != nil {
				anomalies = append(anomalies, Anomaly{
					Reason: "consecutive_in", Punch: open.ID, Kind: open.Kind,
					At: timex.JSONTime{Time: open.At},
				})
				missing = true
			}
			open = p
		case model.KindOut:
			if open == nil {
				anomalies = append(anomalies, Anomaly{
					Reason: "out_without_in", Punch: p.ID, Kind: p.Kind,
					At: timex.JSONTime{Time: p.At},
				})
				missing = true
				continue
			}
			presence = append(presence, timex.Interval{Start: open.At, End: p.At})
			open = nil
		}
	}
	if open != nil {
		anomalies = append(anomalies, Anomaly{
			Reason: "in_without_out", Punch: open.ID, Kind: open.Kind,
			At: timex.JSONTime{Time: open.At},
		})
		missing = true
	}
	return presence, anomalies, missing
}
