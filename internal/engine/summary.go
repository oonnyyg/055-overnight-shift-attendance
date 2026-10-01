package engine

// EmployeeSummary totals one employee's shifts inside the report window.
type EmployeeSummary struct {
	EmployeeID     string `json:"employee_id"`
	ShiftCount     int    `json:"shift_count"`
	RequiredMin    int    `json:"required_minutes"`
	WorkMin        int    `json:"work_minutes"`
	LateMin        int    `json:"late_minutes"`
	EarlyMin       int    `json:"early_minutes"`
	AbsentMin      int    `json:"absent_minutes"`
	MissingShifts  int    `json:"missing_card_shifts"`
	AnomalyRecords int    `json:"anomaly_records"`
}

// Summarize groups per-shift reports into per-employee totals.
// Reports must already be sorted (by employee, then start time).
func Summarize(reports []ShiftReport) []EmployeeSummary {
	var out []EmployeeSummary
	index := map[string]int{}
	for _, r := range reports {
		i, ok := index[r.EmployeeID]
		if !ok {
			out = append(out, EmployeeSummary{EmployeeID: r.EmployeeID})
			i = len(out) - 1
			index[r.EmployeeID] = i
		}
		s := &out[i]
		s.ShiftCount++
		s.RequiredMin += r.RequiredMin
		s.WorkMin += r.WorkMin
		s.LateMin += r.LateMin
		s.EarlyMin += r.EarlyMin
		s.AbsentMin += r.AbsentMin
		if r.MissingCard {
			s.MissingShifts++
		}
		s.AnomalyRecords += len(r.Anomalies)
	}
	return out
}
