package attendance

import "testing"

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func mustAddEmployee(t *testing.T, s *Store, id string) {
	t.Helper()
	if err := s.AddEmployee(Employee{ID: id, Name: id}); err != nil {
		t.Fatal(err)
	}
}

func mustImportShifts(t *testing.T, s *Store, shifts ...Shift) {
	t.Helper()
	if _, err := s.ImportShifts(shifts); err != nil {
		t.Fatal(err)
	}
}

func mustImportPunches(t *testing.T, s *Store, punches ...Punch) {
	t.Helper()
	if _, err := s.ImportPunches(punches); err != nil {
		t.Fatal(err)
	}
}

func detail(t *testing.T, s *Store, shiftID string) ShiftDetail {
	t.Helper()
	d, err := s.ShiftDetail(shiftID)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// 跨夜班次 + 休息 + 打卡，验证迟到/早退/缺勤分解。
func TestOvernightShiftWithBreak(t *testing.T) {
	s := newStore(t)
	mustAddEmployee(t, s, "E1")
	mustImportShifts(t, s, Shift{
		ID: "S1", EmployeeID: "E1",
		Start: "2026-09-01 22:00", End: "2026-09-02 06:00",
		Breaks: []Break{{Start: "2026-09-02 02:00", End: "2026-09-02 02:30"}},
	})
	// 应出勤 480-30=450。迟到 15 分钟，提前 10 分钟离开。
	mustImportPunches(t, s,
		Punch{ID: "P2", EmployeeID: "E1", ShiftID: "S1", Type: PunchOut, Time: "2026-09-02 05:50"},
		Punch{ID: "P1", EmployeeID: "E1", ShiftID: "S1", Type: PunchIn, Time: "2026-09-01 22:15"},
	)
	d := detail(t, s, "S1")
	if d.ExpectedMinutes != 450 {
		t.Fatalf("expected 450, got %d", d.ExpectedMinutes)
	}
	if d.LateMinutes != 15 || d.EarlyMinutes != 10 {
		t.Fatalf("late=%d early=%d", d.LateMinutes, d.EarlyMinutes)
	}
	// 在场 22:15-05:50 = 455，扣休息 30 → 425。
	if d.PresentMinutes != 425 {
		t.Fatalf("present=%d", d.PresentMinutes)
	}
	if d.AbsentMinutes != 0 {
		t.Fatalf("absent=%d", d.AbsentMinutes)
	}
	if d.LateMinutes+d.EarlyMinutes+d.AbsentMinutes+d.PresentMinutes != d.ExpectedMinutes {
		t.Fatal("四类分钟合计不等于应出勤分钟")
	}
}

// 休息重叠取并集，请假只抵扣重叠部分且多张请假只抵扣一次。
func TestBreakUnionAndLeaveOverlap(t *testing.T) {
	s := newStore(t)
	mustAddEmployee(t, s, "E1")
	mustImportShifts(t, s, Shift{
		ID: "S1", EmployeeID: "E1",
		Start: "2026-09-01 09:00", End: "2026-09-01 18:00",
		Breaks: []Break{
			{Start: "2026-09-01 12:00", End: "2026-09-01 13:00"},
			{Start: "2026-09-01 12:30", End: "2026-09-01 13:30"},
		},
	})
	// 休息并集 12:00-13:30 = 90 分钟。
	// 请假1 09:00-11:00，请假2 10:00-12:30（与请假1重叠1小时，且含休息段）。
	if _, err := s.ImportLeaves([]Leave{
		{ID: "L1", EmployeeID: "E1", Start: "2026-09-01 09:00", End: "2026-09-01 11:00"},
		{ID: "L2", EmployeeID: "E1", Start: "2026-09-01 10:00", End: "2026-09-01 12:30"},
	}); err != nil {
		t.Fatal(err)
	}
	d := detail(t, s, "S1")
	// 工作区间 540-90=450。请假并集 09:00-12:30，与工作区间交集 09:00-12:00=180。
	if d.ExpectedMinutes != 450-180 {
		t.Fatalf("expected=%d", d.ExpectedMinutes)
	}
	if len(d.LeaveDeductions) != 1 || d.LeaveDeductions[0].Start != "2026-09-01 09:00" || d.LeaveDeductions[0].End != "2026-09-01 12:00" {
		t.Fatalf("deductions=%+v", d.LeaveDeductions)
	}
	// 无打卡 → 全部缺勤，不算缺卡。
	if d.AbsentMinutes != d.ExpectedMinutes || len(d.Anomalies) != 0 {
		t.Fatalf("absent=%d anomalies=%v", d.AbsentMinutes, d.Anomalies)
	}
}

// 整班请假后应出勤为零仍列出。
func TestFullShiftLeave(t *testing.T) {
	s := newStore(t)
	mustAddEmployee(t, s, "E1")
	mustImportShifts(t, s, Shift{ID: "S1", EmployeeID: "E1", Start: "2026-09-01 22:00", End: "2026-09-02 06:00"})
	if _, err := s.ImportLeaves([]Leave{
		{ID: "L1", EmployeeID: "E1", Start: "2026-09-01 20:00", End: "2026-09-02 08:00"},
	}); err != nil {
		t.Fatal(err)
	}
	d := detail(t, s, "S1")
	if d.ExpectedMinutes != 0 || d.AbsentMinutes != 0 || d.PresentMinutes != 0 {
		t.Fatalf("expected=%d absent=%d present=%d", d.ExpectedMinutes, d.AbsentMinutes, d.PresentMinutes)
	}
	rep, err := s.Report("E1", "2026-09-01", "2026-09-01")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Summary.ShiftCount != 1 {
		t.Fatalf("shift count=%d", rep.Summary.ShiftCount)
	}
}

// 乱序导入、同分钟配对计零分钟、缺卡异常。
func TestOutOfOrderAndMissingPunch(t *testing.T) {
	s := newStore(t)
	mustAddEmployee(t, s, "E1")
	mustImportShifts(t, s,
		Shift{ID: "S1", EmployeeID: "E1", Start: "2026-09-01 09:00", End: "2026-09-01 17:00"},
		Shift{ID: "S2", EmployeeID: "E1", Start: "2026-09-02 09:00", End: "2026-09-02 17:00"},
		Shift{ID: "S3", EmployeeID: "E1", Start: "2026-09-03 09:00", End: "2026-09-03 17:00"},
	)
	// S1: 乱序导入 + 同分钟配对（12:00 进、12:00 出，计零分钟）。
	mustImportPunches(t, s,
		Punch{ID: "P6", EmployeeID: "E1", ShiftID: "S1", Type: PunchOut, Time: "2026-09-01 17:00"},
		Punch{ID: "P2", EmployeeID: "E1", ShiftID: "S1", Type: PunchOut, Time: "2026-09-01 10:00"},
		Punch{ID: "P4", EmployeeID: "E1", ShiftID: "S1", Type: PunchOut, Time: "2026-09-01 12:00"},
		Punch{ID: "P1", EmployeeID: "E1", ShiftID: "S1", Type: PunchIn, Time: "2026-09-01 09:00"},
		Punch{ID: "P5", EmployeeID: "E1", ShiftID: "S1", Type: PunchIn, Time: "2026-09-01 13:00"},
		Punch{ID: "P3", EmployeeID: "E1", ShiftID: "S1", Type: PunchIn, Time: "2026-09-01 12:00"},
	)
	d := detail(t, s, "S1")
	if len(d.Anomalies) != 0 {
		t.Fatalf("anomalies=%v", d.Anomalies)
	}
	// 在场 09:00-10:00(60) + 12:00-12:00(0) + 13:00-17:00(240) = 300。
	if d.PresentMinutes != 300 {
		t.Fatalf("present=%d", d.PresentMinutes)
	}
	// 中间未覆盖 10:00-13:00 = 180 计缺勤。
	if d.AbsentMinutes != 180 || d.LateMinutes != 0 || d.EarlyMinutes != 0 {
		t.Fatalf("absent=%d late=%d early=%d", d.AbsentMinutes, d.LateMinutes, d.EarlyMinutes)
	}
	// S2: 最后缺少离开 → 缺卡，不参与分钟汇总但计入异常。
	mustImportPunches(t, s,
		Punch{ID: "P7", EmployeeID: "E1", ShiftID: "S2", Type: PunchIn, Time: "2026-09-02 09:00"},
	)
	d2 := detail(t, s, "S2")
	if len(d2.Anomalies) != 1 {
		t.Fatalf("anomalies=%v", d2.Anomalies)
	}
	// S3: 开头离开 → 缺卡。
	mustImportPunches(t, s,
		Punch{ID: "P8", EmployeeID: "E1", ShiftID: "S3", Type: PunchOut, Time: "2026-09-03 17:00"},
	)
	d3 := detail(t, s, "S3")
	if len(d3.Anomalies) != 1 {
		t.Fatalf("anomalies=%v", d3.Anomalies)
	}
	rep, err := s.Report("E1", "2026-09-01", "2026-09-03")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Summary.AnomalyCount != 2 {
		t.Fatalf("anomaly count=%d", rep.Summary.AnomalyCount)
	}
	if rep.Summary.PresentMinutes != 300 || rep.Summary.ExpectedMinutes != 480 {
		t.Fatalf("summary=%+v", rep.Summary)
	}
}

// 连续进入标记缺卡。
func TestConsecutiveIns(t *testing.T) {
	s := newStore(t)
	mustAddEmployee(t, s, "E1")
	mustImportShifts(t, s, Shift{ID: "S1", EmployeeID: "E1", Start: "2026-09-01 09:00", End: "2026-09-01 17:00"})
	mustImportPunches(t, s,
		Punch{ID: "P1", EmployeeID: "E1", ShiftID: "S1", Type: PunchIn, Time: "2026-09-01 09:00"},
		Punch{ID: "P2", EmployeeID: "E1", ShiftID: "S1", Type: PunchIn, Time: "2026-09-01 10:00"},
		Punch{ID: "P3", EmployeeID: "E1", ShiftID: "S1", Type: PunchOut, Time: "2026-09-01 17:00"},
	)
	d := detail(t, s, "S1")
	if len(d.Anomalies) == 0 {
		t.Fatal("连续进入应标记缺卡")
	}
}

// 班外打卡：配对正常但在应出勤区间之外，不计加班。
func TestPunchOutsideShift(t *testing.T) {
	s := newStore(t)
	mustAddEmployee(t, s, "E1")
	mustImportShifts(t, s, Shift{ID: "S1", EmployeeID: "E1", Start: "2026-09-01 09:00", End: "2026-09-01 17:00"})
	mustImportPunches(t, s,
		Punch{ID: "P1", EmployeeID: "E1", ShiftID: "S1", Type: PunchIn, Time: "2026-09-01 07:00"},
		Punch{ID: "P2", EmployeeID: "E1", ShiftID: "S1", Type: PunchOut, Time: "2026-09-01 20:00"},
	)
	d := detail(t, s, "S1")
	if d.PresentMinutes != 480 {
		t.Fatalf("present=%d（不应计加班）", d.PresentMinutes)
	}
	if d.LateMinutes != 0 || d.EarlyMinutes != 0 || d.AbsentMinutes != 0 {
		t.Fatalf("late=%d early=%d absent=%d", d.LateMinutes, d.EarlyMinutes, d.AbsentMinutes)
	}
}

// 多班汇总 + 跨夜班次按开始日期归属，不拆分。
func TestMultiShiftSummaryAndDateFilter(t *testing.T) {
	s := newStore(t)
	mustAddEmployee(t, s, "E1")
	mustImportShifts(t, s,
		Shift{ID: "S1", EmployeeID: "E1", Start: "2026-09-01 22:00", End: "2026-09-02 06:00"},
		Shift{ID: "S2", EmployeeID: "E1", Start: "2026-09-02 22:00", End: "2026-09-03 06:00"},
	)
	mustImportPunches(t, s,
		Punch{ID: "P1", EmployeeID: "E1", ShiftID: "S1", Type: PunchIn, Time: "2026-09-01 22:00"},
		Punch{ID: "P2", EmployeeID: "E1", ShiftID: "S1", Type: PunchOut, Time: "2026-09-02 06:00"},
		Punch{ID: "P3", EmployeeID: "E1", ShiftID: "S2", Type: PunchIn, Time: "2026-09-02 22:00"},
		Punch{ID: "P4", EmployeeID: "E1", ShiftID: "S2", Type: PunchOut, Time: "2026-09-03 06:00"},
	)
	rep, err := s.Report("E1", "2026-09-01", "2026-09-01")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Summary.ShiftCount != 1 || rep.Summary.PresentMinutes != 480 {
		t.Fatalf("summary=%+v", rep.Summary)
	}
	rep, err = s.Report("E1", "2026-09-01", "2026-09-02")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Summary.ShiftCount != 2 || rep.Summary.PresentMinutes != 960 {
		t.Fatalf("summary=%+v", rep.Summary)
	}
}

// 重开后结果一致。
func TestReopenConsistency(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	mustAddEmployee(t, s, "E1")
	mustImportShifts(t, s, Shift{ID: "S1", EmployeeID: "E1", Start: "2026-09-01 22:00", End: "2026-09-02 06:00"})
	mustImportPunches(t, s,
		Punch{ID: "P1", EmployeeID: "E1", ShiftID: "S1", Type: PunchIn, Time: "2026-09-01 22:00"},
		Punch{ID: "P2", EmployeeID: "E1", ShiftID: "S1", Type: PunchOut, Time: "2026-09-02 06:00"},
	)
	before, err := s.Report("E1", "2026-09-01", "2026-09-01")
	if err != nil {
		t.Fatal(err)
	}
	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	after, err := s2.Report("E1", "2026-09-01", "2026-09-01")
	if err != nil {
		t.Fatal(err)
	}
	if before.Summary != after.Summary || len(after.Shifts) != 1 {
		t.Fatalf("重开后不一致: %+v vs %+v", before.Summary, after.Summary)
	}
}
