package timex

import "time"

// Layout is the canonical date-time format used everywhere.
const Layout = "2006-01-02 15:04"

// Beijing is the timezone all input and output is interpreted in.
var Beijing = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	return loc
}()

// Parse parses a "YYYY-MM-DD HH:mm" string in Beijing time.
func Parse(s string) (time.Time, error) {
	t, err := time.ParseInLocation(Layout, s, Beijing)
	if err != nil {
		return time.Time{}, err
	}
	return t, nil
}

// Format formats t as "YYYY-MM-DD HH:mm" in Beijing time.
func Format(t time.Time) string {
	return t.In(Beijing).Format(Layout)
}

// Date renders t as "YYYY-MM-DD" in Beijing time.
func Date(t time.Time) string {
	return t.In(Beijing).Format("2006-01-02")
}
