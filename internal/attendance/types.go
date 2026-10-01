package attendance

import (
	"fmt"
	"time"
)

// TimeLayout 是统一的北京时间时间格式。
const TimeLayout = "2006-01-02 15:04"

// DateLayout 是报告筛选使用的日期格式。
const DateLayout = "2006-01-02"

// Beijing 是固定的北京时间时区（UTC+8）。
var Beijing = time.FixedZone("Asia/Shanghai", 8*3600)

// Interval 表示左闭右开的时间区间 [Start, End)。
type Interval struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Minutes 返回区间分钟数，非法区间返回 0。
func (iv Interval) Minutes() int {
	d := int(iv.End.Sub(iv.Start).Minutes())
	if d < 0 {
		return 0
	}
	return d
}

// ParseTime 解析 YYYY-MM-DD HH:mm 格式的北京时间。
func ParseTime(s string) (time.Time, error) {
	t, err := time.ParseInLocation(TimeLayout, s, Beijing)
	if err != nil {
		return time.Time{}, fmt.Errorf("非法时间 %q，应为 YYYY-MM-DD HH:mm", s)
	}
	return t, nil
}

// ParseDate 解析 YYYY-MM-DD 日期（北京时间当日零点）。
func ParseDate(s string) (time.Time, error) {
	t, err := time.ParseInLocation(DateLayout, s, Beijing)
	if err != nil {
		return time.Time{}, fmt.Errorf("非法日期 %q，应为 YYYY-MM-DD", s)
	}
	return t, nil
}

// FormatTime 以统一格式输出时间。
func FormatTime(t time.Time) string {
	return t.In(Beijing).Format(TimeLayout)
}

// Employee 员工。
type Employee struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Break 班次内的休息区间。
type Break struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// Shift 班次，可跨午夜，起止不得相同。
type Shift struct {
	ID         string  `json:"id"`
	EmployeeID string  `json:"employee_id"`
	Start      string  `json:"start"`
	End        string  `json:"end"`
	Breaks     []Break `json:"breaks,omitempty"`
}

// Leave 已批准请假。
type Leave struct {
	ID         string `json:"id"`
	EmployeeID string `json:"employee_id"`
	Start      string `json:"start"`
	End        string `json:"end"`
}

// PunchType 打卡方向。
type PunchType string

const (
	PunchIn  PunchType = "in"
	PunchOut PunchType = "out"
)

// Punch 打卡记录，归属员工与班次。
type Punch struct {
	ID         string    `json:"id"`
	EmployeeID string    `json:"employee_id"`
	ShiftID    string    `json:"shift_id"`
	Type       PunchType `json:"type"`
	Time       string    `json:"time"`
}

// Data 是持久化的全部数据。
type Data struct {
	Employees map[string]Employee `json:"employees"`
	Shifts    map[string]Shift    `json:"shifts"`
	Leaves    map[string]Leave    `json:"leaves"`
	Punches   map[string]Punch    `json:"punches"`
}

func newData() *Data {
	return &Data{
		Employees: map[string]Employee{},
		Shifts:    map[string]Shift{},
		Leaves:    map[string]Leave{},
		Punches:   map[string]Punch{},
	}
}
