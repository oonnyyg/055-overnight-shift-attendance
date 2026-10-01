package attendance

import (
	"strings"
	"testing"
)

func TestUnknownEmployeeAndShift(t *testing.T) {
	s := newStore(t)
	if _, err := s.ImportShifts([]Shift{{ID: "S1", EmployeeID: "ghost", Start: "2026-09-01 09:00", End: "2026-09-01 17:00"}}); err == nil {
		t.Fatal("未知员工应被拒绝")
	}
	mustAddEmployee(t, s, "E1")
	if _, err := s.ImportPunches([]Punch{{ID: "P1", EmployeeID: "E1", ShiftID: "ghost", Type: PunchIn, Time: "2026-09-01 09:00"}}); err == nil {
		t.Fatal("未知班次应被拒绝")
	}
	if _, err := s.Report("ghost", "2026-09-01", "2026-09-02"); err == nil {
		t.Fatal("未知员工报告应被拒绝")
	}
}

func TestInvalidTimeAndReversedRange(t *testing.T) {
	s := newStore(t)
	mustAddEmployee(t, s, "E1")
	if _, err := s.ImportShifts([]Shift{{ID: "S1", EmployeeID: "E1", Start: "2026-09-01 17:00", End: "2026-09-01 09:00"}}); err == nil {
		t.Fatal("起止倒置应被拒绝")
	}
	if _, err := s.ImportShifts([]Shift{{ID: "S1", EmployeeID: "E1", Start: "2026-09-01 09:00", End: "2026-09-01 09:00"}}); err == nil {
		t.Fatal("起止相同应被拒绝")
	}
	if _, err := s.ImportShifts([]Shift{{ID: "S1", EmployeeID: "E1", Start: "09:00", End: "2026-09-01 17:00"}}); err == nil {
		t.Fatal("非法时间应被拒绝")
	}
	if _, err := s.ImportLeaves([]Leave{{ID: "L1", EmployeeID: "E1", Start: "2026-09-02 09:00", End: "2026-09-01 09:00"}}); err == nil {
		t.Fatal("请假起止倒置应被拒绝")
	}
	if _, err := s.Report("E1", "2026-09-03", "2026-09-01"); err == nil {
		t.Fatal("报告日期倒置应被拒绝")
	}
}

func TestBreakOutsideShift(t *testing.T) {
	s := newStore(t)
	mustAddEmployee(t, s, "E1")
	sh := Shift{ID: "S1", EmployeeID: "E1", Start: "2026-09-01 09:00", End: "2026-09-01 17:00",
		Breaks: []Break{{Start: "2026-09-01 16:30", End: "2026-09-01 18:00"}}}
	if _, err := s.ImportShifts([]Shift{sh}); err == nil {
		t.Fatal("休息超出班次应被拒绝")
	}
}

func TestShiftOverlap(t *testing.T) {
	s := newStore(t)
	mustAddEmployee(t, s, "E1")
	mustAddEmployee(t, s, "E2")
	mustImportShifts(t, s, Shift{ID: "S1", EmployeeID: "E1", Start: "2026-09-01 22:00", End: "2026-09-02 06:00"})
	// 与跨夜班次重叠。
	if _, err := s.ImportShifts([]Shift{{ID: "S2", EmployeeID: "E1", Start: "2026-09-02 05:00", End: "2026-09-02 09:00"}}); err == nil {
		t.Fatal("重叠班次应被拒绝")
	}
	// 批次内重叠。
	if _, err := s.ImportShifts([]Shift{
		{ID: "S3", EmployeeID: "E1", Start: "2026-09-03 09:00", End: "2026-09-03 17:00"},
		{ID: "S4", EmployeeID: "E1", Start: "2026-09-03 12:00", End: "2026-09-03 20:00"},
	}); err == nil {
		t.Fatal("批次内重叠应被拒绝")
	}
	// 不同员工不冲突。
	mustImportShifts(t, s, Shift{ID: "S5", EmployeeID: "E2", Start: "2026-09-01 22:00", End: "2026-09-02 06:00"})
	// 首尾相接不算重叠（左闭右开）。
	mustImportShifts(t, s, Shift{ID: "S6", EmployeeID: "E1", Start: "2026-09-02 06:00", End: "2026-09-02 12:00"})
}

func TestDuplicateIDConflictAndIdempotentReimport(t *testing.T) {
	s := newStore(t)
	mustAddEmployee(t, s, "E1")
	if err := s.AddEmployee(Employee{ID: "E1", Name: "E1"}); err != nil {
		t.Fatal("相同员工重复导入应被忽略")
	}
	if err := s.AddEmployee(Employee{ID: "E1", Name: "别的名字"}); err == nil {
		t.Fatal("同标识不同内容应冲突")
	}
	sh := Shift{ID: "S1", EmployeeID: "E1", Start: "2026-09-01 09:00", End: "2026-09-01 17:00"}
	mustImportShifts(t, s, sh)
	n, err := s.ImportShifts([]Shift{sh})
	if err != nil || n != 0 {
		t.Fatalf("相同班次重复导入不应增加记录: n=%d err=%v", n, err)
	}
	if len(s.data.Shifts) != 1 {
		t.Fatalf("shifts=%d", len(s.data.Shifts))
	}
	other := sh
	other.End = "2026-09-01 18:00"
	if _, err := s.ImportShifts([]Shift{other}); err == nil {
		t.Fatal("同标识不同班次应冲突")
	}
}

// 批量导入失败不得留下部分记录。
func TestBatchAtomicity(t *testing.T) {
	s := newStore(t)
	mustAddEmployee(t, s, "E1")
	_, err := s.ImportShifts([]Shift{
		{ID: "S1", EmployeeID: "E1", Start: "2026-09-01 09:00", End: "2026-09-01 17:00"},
		{ID: "S2", EmployeeID: "ghost", Start: "2026-09-02 09:00", End: "2026-09-02 17:00"},
	})
	if err == nil {
		t.Fatal("含未知员工的批次应失败")
	}
	if len(s.data.Shifts) != 0 {
		t.Fatalf("失败批次留下部分记录: %d", len(s.data.Shifts))
	}
	// 批次内标识重复。
	if _, err := s.ImportLeaves([]Leave{
		{ID: "L1", EmployeeID: "E1", Start: "2026-09-01 09:00", End: "2026-09-01 10:00"},
		{ID: "L1", EmployeeID: "E1", Start: "2026-09-01 11:00", End: "2026-09-01 12:00"},
	}); err == nil {
		t.Fatal("批次内重复标识应失败")
	}
	if len(s.data.Leaves) != 0 {
		t.Fatal("失败批次留下部分请假记录")
	}
}

func TestPunchEmployeeMustMatchShift(t *testing.T) {
	s := newStore(t)
	mustAddEmployee(t, s, "E1")
	mustAddEmployee(t, s, "E2")
	mustImportShifts(t, s, Shift{ID: "S1", EmployeeID: "E1", Start: "2026-09-01 09:00", End: "2026-09-01 17:00"})
	if _, err := s.ImportPunches([]Punch{{ID: "P1", EmployeeID: "E2", ShiftID: "S1", Type: PunchIn, Time: "2026-09-01 09:00"}}); err == nil {
		t.Fatal("员工与班次不一致应被拒绝")
	}
	if _, err := s.ImportPunches([]Punch{{ID: "P1", EmployeeID: "E1", ShiftID: "S1", Type: "sideways", Time: "2026-09-01 09:00"}}); err == nil {
		t.Fatal("非法打卡类型应被拒绝")
	}
}

func TestErrorMessagesAreChinese(t *testing.T) {
	s := newStore(t)
	_, err := s.ImportShifts([]Shift{{ID: "S1", EmployeeID: "ghost", Start: "2026-09-01 09:00", End: "2026-09-01 17:00"}})
	if err == nil || !strings.Contains(err.Error(), "未知员工") {
		t.Fatalf("err=%v", err)
	}
}
