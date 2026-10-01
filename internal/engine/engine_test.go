package engine

import (
	"testing"
	"time"

	"example.com/overnight-shift-attendance/internal/model"
	"example.com/overnight-shift-attendance/internal/timex"
)

func at(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := timex.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func br(t *testing.T, a, b string) timex.Interval {
	return timex.Interval{Start: at(t, a), End: at(t, b)}
}

func TestFullAttendanceNightShift(t *testing.T) {
	sh := model.Shift{
		ID: "s1", EmpID: "e1",
		Start:  at(t, "2026-10-01 22:00"),
		End:    at(t, "2026-10-02 06:00"),
		Breaks: []timex.Interval{br(t, "2026-10-02 02:00", "2026-10-02 02:30")},
	}
	punches := []model.Punch{
		{ID: "p1", EmpID: "e1", ShiftID: "s1", Kind: model.KindIn, At: sh.Start},
		{ID: "p2", EmpID: "e1", ShiftID: "s1", Kind: model.KindOut, At: sh.End},
	}
	rep := Build(sh, nil, punches)
	if rep.RequiredMin != 8*60-30 {
		t.Fatalf("required = %d", rep.RequiredMin)
	}
	if rep.WorkMin != rep.RequiredMin {
		t.Fatalf("work = %d want %d", rep.WorkMin, rep.RequiredMin)
	}
	if rep.LateMin+rep.EarlyMin+rep.AbsentMin != 0 || rep.MissingCard {
		t.Fatalf("unexpected anomalies: %+v", rep)
	}
	if rep.Date != "2026-10-01" {
		t.Fatalf("date attributed to %s", rep.Date)
	}
}

func TestLateEarlyAndMiddleGap(t *testing.T) {
	sh := model.Shift{
		ID: "s1", EmpID: "e1",
		Start: at(t, "2026-10-01 09:00"),
		End:   at(t, "2026-10-01 18:00"),
	}
	punches := []model.Punch{
		{ID: "p1", Kind: model.KindIn, At: at(t, "2026-10-01 09:10")},
		{ID: "p2", Kind: model.KindOut, At: at(t, "2026-10-01 12:00")},
		{ID: "p3", Kind: model.KindIn, At: at(t, "2026-10-01 13:00")},
		{ID: "p4", Kind: model.KindOut, At: at(t, "2026-10-01 17:30")},
	}
	rep := Build(sh, nil, punches)
	if rep.LateMin != 10 || rep.EarlyMin != 30 || rep.AbsentMin != 60 {
		t.Fatalf("late=%d early=%d absent=%d", rep.LateMin, rep.EarlyMin, rep.AbsentMin)
	}
	if rep.WorkMin != 9*60-100 {
		t.Fatalf("work = %d", rep.WorkMin)
	}
	if rep.WorkMin+rep.LateMin+rep.EarlyMin+rep.AbsentMin != rep.RequiredMin {
		t.Fatal("minute totals must equal required minutes")
	}
}

func TestSameMinutePairAndNoPunches(t *testing.T) {
	sh := model.Shift{ID: "s1", EmpID: "e1",
		Start: at(t, "2026-10-01 09:00"), End: at(t, "2026-10-01 18:00")}
	rep := Build(sh, nil, []model.Punch{
		{ID: "p1", Kind: model.KindIn, At: at(t, "2026-10-01 10:00")},
		{ID: "p2", Kind: model.KindOut, At: at(t, "2026-10-01 10:00")},
	})
	if rep.MissingCard || rep.WorkMin != 0 || rep.AbsentMin != rep.RequiredMin {
		t.Fatalf("same-minute pair: %+v", rep)
	}

	rep = Build(sh, nil, nil)
	if rep.MissingCard || rep.AbsentMin != rep.RequiredMin || rep.WorkMin != 0 {
		t.Fatalf("no punches: %+v", rep)
	}
}

func TestMissingCardPatterns(t *testing.T) {
	sh := model.Shift{ID: "s1", EmpID: "e1",
		Start: at(t, "2026-10-01 09:00"), End: at(t, "2026-10-01 18:00")}
	cases := map[string][]model.Punch{
		"leading_out": {{ID: "p1", Kind: model.KindOut, At: sh.Start}},
		"double_in": {
			{ID: "p1", Kind: model.KindIn, At: at(t, "2026-10-01 09:00")},
			{ID: "p2", Kind: model.KindIn, At: at(t, "2026-10-01 10:00")},
			{ID: "p3", Kind: model.KindOut, At: at(t, "2026-10-01 18:00")},
		},
		"trailing_in": {{ID: "p1", Kind: model.KindIn, At: sh.Start}},
		"double_out": {
			{ID: "p1", Kind: model.KindIn, At: at(t, "2026-10-01 09:00")},
			{ID: "p2", Kind: model.KindOut, At: at(t, "2026-10-01 17:00")},
			{ID: "p3", Kind: model.KindOut, At: at(t, "2026-10-01 18:00")},
		},
	}
	for name, punches := range cases {
		rep := Build(sh, nil, punches)
		if !rep.MissingCard {
			t.Fatalf("%s: expected missing card", name)
		}
		if rep.WorkMin != 0 || rep.LateMin != 0 || rep.EarlyMin != 0 ||
			rep.AbsentMin != rep.RequiredMin || len(rep.Anomalies) == 0 {
			t.Fatalf("%s: bad summary %+v", name, rep)
		}
	}
}

func TestWholeShiftLeaveAndOverlap(t *testing.T) {
	sh := model.Shift{ID: "s1", EmpID: "e1",
		Start: at(t, "2026-10-01 09:00"), End: at(t, "2026-10-01 12:00")}
	leaves := []model.Leave{
		{ID: "l1", EmpID: "e1",
			Start: at(t, "2026-10-01 08:00"), End: at(t, "2026-10-01 10:30")},
		{ID: "l2", EmpID: "e1",
			Start: at(t, "2026-10-01 10:00"), End: at(t, "2026-10-01 13:00")},
	}
	rep := Build(sh, leaves, nil)
	if rep.RequiredMin != 0 {
		t.Fatalf("fully-leaved required = %d", rep.RequiredMin)
	}
	if rep.WorkMin+rep.AbsentMin+rep.LateMin+rep.EarlyMin != 0 {
		t.Fatalf("zero-required shift must stay zero: %+v", rep)
	}
	if len(rep.Leaves) != 1 {
		t.Fatalf("overlapping leaves should merge to one deduction, got %d", len(rep.Leaves))
	}
}

func TestLeaveDeductedOnceWithBreaks(t *testing.T) {
	sh := model.Shift{
		ID: "s1", EmpID: "e1",
		Start:  at(t, "2026-10-01 09:00"),
		End:    at(t, "2026-10-01 18:00"),
		Breaks: []timex.Interval{br(t, "2026-10-01 12:00", "2026-10-01 13:00")},
	}
	leaves := []model.Leave{{ID: "l1", EmpID: "e1",
		Start: at(t, "2026-10-01 10:00"), End: at(t, "2026-10-01 11:00")}}
	punches := []model.Punch{
		{ID: "p1", Kind: model.KindIn, At: at(t, "2026-10-01 09:00")},
		{ID: "p2", Kind: model.KindOut, At: at(t, "2026-10-01 18:00")},
	}
	rep := Build(sh, leaves, punches)
	if rep.RequiredMin != 8*60-60 {
		t.Fatalf("required = %d", rep.RequiredMin)
	}
	if rep.WorkMin != rep.RequiredMin {
		t.Fatalf("leave-covered work = %d want %d", rep.WorkMin, rep.RequiredMin)
	}
}
