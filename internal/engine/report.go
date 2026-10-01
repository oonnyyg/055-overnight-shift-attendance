package engine

import (
	"sort"
	"time"

	"example.com/overnight-shift-attendance/internal/model"
	"example.com/overnight-shift-attendance/internal/timex"
)

// Build creates a report for one shift using all leaves and punches of its
// employee. Punches belonging to other shifts are ignored here; callers pass
// punches already filtered to this shift.
func Build(sh model.Shift, leaves []model.Leave, punches []model.Punch) ShiftReport {
	rep := ShiftReport{
		ShiftID:    sh.ID,
		EmployeeID: sh.EmpID,
		Date:       timex.Date(sh.Start),
		ShiftStart: timex.JSONTime{Time: sh.Start},
		ShiftEnd:   timex.JSONTime{Time: sh.End},
	}

	work := timex.Interval{Start: sh.Start, End: sh.End}
	rep.Breaks = toJSON(sh.Breaks)

	var leaveIvs []timex.Interval
	for _, lv := range leaves {
		if lv.EmpID != sh.EmpID {
			continue
		}
		if ov := timex.Intersect(work, timex.Interval{Start: lv.Start, End: lv.End}); ov.Valid() {
			leaveIvs = append(leaveIvs, ov)
		}
	}
	leaveIvs = timex.Merge(leaveIvs)
	rep.Leaves = toJSON(leaveIvs)

	required := timex.SubtractAll(work, append(append([]timex.Interval{}, sh.Breaks...), leaveIvs...))
	required = timex.Merge(required)
	rep.Required = toJSON(required)
	rep.RequiredMin = timex.SumMinutes(required)

	sortedPunches := make([]model.Punch, len(punches))
	copy(sortedPunches, punches)
	sort.SliceStable(sortedPunches, func(i, j int) bool {
		if sortedPunches[i].At.Equal(sortedPunches[j].At) {
			return sortedPunches[i].ID < sortedPunches[j].ID
		}
		return sortedPunches[i].At.Before(sortedPunches[j].At)
	})
	for _, p := range sortedPunches {
		rep.Punches = append(rep.Punches, PunchJSON{ID: p.ID, Kind: p.Kind,
			At: timex.JSONTime{Time: p.At}})
	}

	presence, anomalies, missing := PairPunches(punches)
	rep.Anomalies = anomalies
	rep.MissingCard = missing
	if missing {
		rep.AbsentMin = rep.RequiredMin
		return rep
	}

	presence = timex.Merge(presence)
	rep.Presence = toJSON(presence)

	var effective []timex.Interval
	for _, req := range required {
		effective = append(effective, timex.IntersectAll(req, presence)...)
	}
	effective = timex.Merge(effective)

	if len(effective) == 0 {
		rep.AbsentMin = rep.RequiredMin
		return rep
	}

	first := effective[0].Start
	last := effective[len(effective)-1].End

	for _, req := range required {
		lateIv := timex.Interval{}
		earlyIv := timex.Interval{}
		if req.Start.Before(first) {
			lateIv = timex.Interval{Start: req.Start, End: minT(req.End, first)}
			rep.LateMin += lateIv.Minutes()
		}
		if req.End.After(last) {
			earlyIv = timex.Interval{Start: maxT(req.Start, last), End: req.End}
			rep.EarlyMin += earlyIv.Minutes()
		}
		covered := timex.IntersectAll(req, effective)
		rest := timex.SubtractAll(req, covered)
		if lateIv.Valid() {
			rest = timex.SubtractFromAll(rest, lateIv)
		}
		if earlyIv.Valid() {
			rest = timex.SubtractFromAll(rest, earlyIv)
		}
		rep.AbsentMin += timex.SumMinutes(rest)
	}
	rep.WorkMin = timex.SumMinutes(effective)

	total := rep.WorkMin + rep.LateMin + rep.EarlyMin + rep.AbsentMin
	if total != rep.RequiredMin {
		rep.AbsentMin += rep.RequiredMin - total
	}
	return rep
}

func toJSON(ivs []timex.Interval) []IntervalJSON {
	merged := timex.Merge(ivs)
	if len(merged) == 0 {
		return []IntervalJSON{}
	}
	out := make([]IntervalJSON, len(merged))
	for i, iv := range merged {
		out[i] = IntervalJSON{Start: timex.Format(iv.Start), End: timex.Format(iv.End)}
	}
	return out
}

func minT(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func maxT(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
