package attendance

import (
	"fmt"
	"sort"
	"time"
)

// IntervalJSON 是输出用的区间。
type IntervalJSON struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

func toJSON(ivs []Interval) []IntervalJSON {
	out := make([]IntervalJSON, 0, len(ivs))
	for _, iv := range ivs {
		out = append(out, IntervalJSON{Start: FormatTime(iv.Start), End: FormatTime(iv.End)})
	}
	return out
}

// PunchJSON 是输出用的打卡记录。
type PunchJSON struct {
	ID   string    `json:"id"`
	Type PunchType `json:"type"`
	Time string    `json:"time"`
}

// ShiftDetail 是单班次的完整明细。
type ShiftDetail struct {
	ShiftID         string         `json:"shift_id"`
	EmployeeID      string         `json:"employee_id"`
	Start           string         `json:"start"`
	End             string         `json:"end"`
	Breaks          []IntervalJSON `json:"breaks"`
	Punches         []PunchJSON    `json:"punches"`
	LeaveDeductions []IntervalJSON `json:"leave_deductions"`
	ExpectedSpans   []IntervalJSON `json:"expected_spans"`
	ExpectedMinutes int            `json:"expected_minutes"`
	PresentMinutes  int            `json:"present_minutes"`
	LateMinutes     int            `json:"late_minutes"`
	EarlyMinutes    int            `json:"early_minutes"`
	AbsentMinutes   int            `json:"absent_minutes"`
	Anomalies       []string       `json:"anomalies"`
}

// Summary 是期间汇总。
type Summary struct {
	EmployeeID      string `json:"employee_id"`
	From            string `json:"from"`
	To              string `json:"to"`
	ShiftCount      int    `json:"shift_count"`
	AnomalyCount    int    `json:"anomaly_count"`
	ExpectedMinutes int    `json:"expected_minutes"`
	PresentMinutes  int    `json:"present_minutes"`
	LateMinutes     int    `json:"late_minutes"`
	EarlyMinutes    int    `json:"early_minutes"`
	AbsentMinutes   int    `json:"absent_minutes"`
}

// Report 是期间报告。
type Report struct {
	Summary Summary       `json:"summary"`
	Shifts  []ShiftDetail `json:"shifts"`
}

// sortedPunches 按时刻再按标识排序。
func sortedPunches(punches []Punch) []Punch {
	out := append([]Punch(nil), punches...)
	sort.Slice(out, func(i, j int) bool {
		ti, _ := ParseTime(out[i].Time)
		tj, _ := ParseTime(out[j].Time)
		if !ti.Equal(tj) {
			return ti.Before(tj)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// pairPunches 配对打卡，返回在场区间与异常描述。
func pairPunches(sorted []Punch) ([]Interval, []string) {
	var presence []Interval
	var anomalies []string
	expectIn := true
	var openIn Punch
	openTime := time.Time{}
	for _, p := range sorted {
		t, _ := ParseTime(p.Time)
		if expectIn {
			if p.Type == PunchOut {
				anomalies = append(anomalies, fmt.Sprintf("打卡 %s（%s 离开）缺少对应进入", p.ID, p.Time))
				continue
			}
			openIn = p
			openTime = t
			expectIn = false
			continue
		}
		if p.Type == PunchIn {
			anomalies = append(anomalies, fmt.Sprintf("打卡 %s（%s 进入）前缺少离开", p.ID, p.Time))
			continue
		}
		presence = append(presence, Interval{Start: openTime, End: t})
		expectIn = true
	}
	if !expectIn {
		anomalies = append(anomalies, fmt.Sprintf("打卡 %s（%s 进入）后缺少离开", openIn.ID, openIn.Time))
	}
	return presence, anomalies
}

// buildDetail 计算单班次明细。
func (s *Store) buildDetail(sh Shift) (ShiftDetail, error) {
	span, err := shiftInterval(sh)
	if err != nil {
		return ShiftDetail{}, err
	}
	breaks, err := shiftBreaks(sh, span)
	if err != nil {
		return ShiftDetail{}, err
	}
	work := subtractIntervals([]Interval{span}, breaks)

	// 请假只抵扣与班次工作区间重叠的部分，重叠请假只抵扣一次。
	var leaveCuts []Interval
	for _, lv := range s.data.Leaves {
		if lv.EmployeeID != sh.EmployeeID {
			continue
		}
		start, _ := ParseTime(lv.Start)
		end, _ := ParseTime(lv.End)
		leaveCuts = append(leaveCuts, Interval{Start: start, End: end})
	}
	deducted := intersectIntervals(work, leaveCuts)
	expected := subtractIntervals(work, deducted)

	var punches []Punch
	for _, p := range s.data.Punches {
		if p.ShiftID == sh.ID {
			punches = append(punches, p)
		}
	}
	sorted := sortedPunches(punches)
	presence, anomalies := pairPunches(sorted)
	if anomalies == nil {
		anomalies = []string{}
	}

	detail := ShiftDetail{
		ShiftID:         sh.ID,
		EmployeeID:      sh.EmployeeID,
		Start:           FormatTime(span.Start),
		End:             FormatTime(span.End),
		Breaks:          toJSON(breaks),
		LeaveDeductions: toJSON(deducted),
		ExpectedSpans:   toJSON(expected),
		ExpectedMinutes: totalMinutes(expected),
		Anomalies:       anomalies,
	}
	for _, p := range sorted {
		detail.Punches = append(detail.Punches, PunchJSON{ID: p.ID, Type: p.Type, Time: p.Time})
	}
	if detail.Punches == nil {
		detail.Punches = []PunchJSON{}
	}

	if len(anomalies) > 0 {
		// 缺卡班次不参与分钟汇总。
		return detail, nil
	}
	effective := intersectIntervals(presence, expected)
	detail.PresentMinutes = totalMinutes(effective)
	expectedTotal := detail.ExpectedMinutes
	if len(effective) == 0 {
		detail.AbsentMinutes = expectedTotal
		return detail, nil
	}
	first := effective[0].Start
	last := effective[len(effective)-1].End
	past := time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)
	future := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
	late := intersectIntervals(expected, []Interval{{Start: past, End: first}})
	early := intersectIntervals(expected, []Interval{{Start: last, End: future}})
	middle := intersectIntervals(expected, []Interval{{Start: first, End: last}})
	absent := subtractIntervals(middle, effective)
	detail.LateMinutes = totalMinutes(late)
	detail.EarlyMinutes = totalMinutes(early)
	detail.AbsentMinutes = totalMinutes(absent)
	return detail, nil
}

// ShiftDetail 查询单班次明细。
func (s *Store) ShiftDetail(shiftID string) (ShiftDetail, error) {
	sh, ok := s.data.Shifts[shiftID]
	if !ok {
		return ShiftDetail{}, fmt.Errorf("未知班次 %q", shiftID)
	}
	return s.buildDetail(sh)
}

// Report 生成员工在日期范围内的报告，按班次开始日期归属，含起止两端。
func (s *Store) Report(employeeID, from, to string) (Report, error) {
	if _, ok := s.data.Employees[employeeID]; !ok {
		return Report{}, fmt.Errorf("未知员工 %q", employeeID)
	}
	fromDate, err := ParseDate(from)
	if err != nil {
		return Report{}, err
	}
	toDate, err := ParseDate(to)
	if err != nil {
		return Report{}, err
	}
	if toDate.Before(fromDate) {
		return Report{}, fmt.Errorf("日期范围起止倒置")
	}
	rep := Report{
		Summary: Summary{EmployeeID: employeeID, From: from, To: to},
		Shifts:  []ShiftDetail{},
	}
	var shifts []Shift
	for _, sh := range s.data.Shifts {
		if sh.EmployeeID != employeeID {
			continue
		}
		span, err := shiftInterval(sh)
		if err != nil {
			continue
		}
		day := time.Date(span.Start.Year(), span.Start.Month(), span.Start.Day(), 0, 0, 0, 0, Beijing)
		if day.Before(fromDate) || day.After(toDate) {
			continue
		}
		shifts = append(shifts, sh)
	}
	sort.Slice(shifts, func(i, j int) bool {
		si, _ := ParseTime(shifts[i].Start)
		sj, _ := ParseTime(shifts[j].Start)
		if !si.Equal(sj) {
			return si.Before(sj)
		}
		return shifts[i].ID < shifts[j].ID
	})
	for _, sh := range shifts {
		detail, err := s.buildDetail(sh)
		if err != nil {
			return Report{}, err
		}
		rep.Shifts = append(rep.Shifts, detail)
		rep.Summary.ShiftCount++
		if len(detail.Anomalies) > 0 {
			rep.Summary.AnomalyCount++
			continue
		}
		rep.Summary.ExpectedMinutes += detail.ExpectedMinutes
		rep.Summary.PresentMinutes += detail.PresentMinutes
		rep.Summary.LateMinutes += detail.LateMinutes
		rep.Summary.EarlyMinutes += detail.EarlyMinutes
		rep.Summary.AbsentMinutes += detail.AbsentMinutes
	}
	return rep, nil
}
