package timex

import "time"

// Interval is a half-open interval [Start, End). Zero-value Interval is empty.
type Interval struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Minutes returns the length of the interval in whole minutes.
func (iv Interval) Minutes() int {
	if !iv.Valid() {
		return 0
	}
	return int(iv.End.Sub(iv.Start) / time.Minute)
}

// Valid reports whether the interval is non-empty.
func (iv Interval) Valid() bool {
	return !iv.Start.IsZero() && iv.End.After(iv.Start)
}

// Contains reports whether t falls inside [Start, End).
func (iv Interval) Contains(t time.Time) bool {
	return !t.Before(iv.Start) && t.Before(iv.End)
}

// Overlaps reports whether two half-open intervals share any point.
func Overlaps(a, b Interval) bool {
	return a.Start.Before(b.End) && b.Start.Before(a.End)
}

// Intersect returns the overlap of a and b (may be empty).
func Intersect(a, b Interval) Interval {
	s := maxTime(a.Start, b.Start)
	e := minTime(a.End, b.End)
	if !e.After(s) {
		return Interval{}
	}
	return Interval{Start: s, End: e}
}

// Union merges a and b if they overlap or touch; otherwise returns false.
func Union(a, b Interval) (Interval, bool) {
	if a.Start.After(b.End) || b.Start.After(a.End) {
		return Interval{}, false
	}
	return Interval{Start: minTime(a.Start, b.Start), End: maxTime(a.End, b.End)}, true
}

// Merge merges all overlapping or touching intervals. Input order irrelevant.
func Merge(ivs []Interval) []Interval {
	if len(ivs) == 0 {
		return nil
	}
	sorted := make([]Interval, len(ivs))
	copy(sorted, ivs)
	sortIntervals(sorted)
	out := []Interval{sorted[0]}
	for _, iv := range sorted[1:] {
		if merged, ok := Union(out[len(out)-1], iv); ok {
			out[len(out)-1] = merged
		} else {
			out = append(out, iv)
		}
	}
	return out
}

// Subtract removes b from a, returning zero or more intervals.
func Subtract(a, b Interval) []Interval {
	if !a.Valid() {
		return nil
	}
	ov := Intersect(a, b)
	if !ov.Valid() {
		return []Interval{a}
	}
	var out []Interval
	if a.Start.Before(ov.Start) {
		out = append(out, Interval{Start: a.Start, End: ov.Start})
	}
	if ov.End.Before(a.End) {
		out = append(out, Interval{Start: ov.End, End: a.End})
	}
	return out
}

// SubtractAll removes every interval in bs (merged first) from a.
func SubtractAll(a Interval, bs []Interval) []Interval {
	remaining := []Interval{a}
	for _, cut := range Merge(bs) {
		var next []Interval
		for _, r := range remaining {
			next = append(next, Subtract(r, cut)...)
		}
		remaining = next
	}
	return remaining
}

// IntersectAll keeps only the parts of a covered by any interval in bs.
func IntersectAll(a Interval, bs []Interval) []Interval {
	var out []Interval
	for _, b := range Merge(bs) {
		if ov := Intersect(a, b); ov.Valid() {
			out = append(out, ov)
		}
	}
	return Merge(out)
}

// SubtractFromAll removes b from every interval in as.
func SubtractFromAll(as []Interval, b Interval) []Interval {
	var out []Interval
	for _, a := range as {
		out = append(out, Subtract(a, b)...)
	}
	return out
}

// SumMinutes totals the lengths of the intervals in minutes.
func SumMinutes(ivs []Interval) int {
	total := 0
	for _, iv := range Merge(ivs) {
		total += iv.Minutes()
	}
	return total
}

func sortIntervals(ivs []Interval) {
	for i := 1; i < len(ivs); i++ {
		for j := i; j > 0 && (ivs[j].Start.Before(ivs[j-1].Start) ||
			(ivs[j].Start.Equal(ivs[j-1].Start) && ivs[j].End.Before(ivs[j-1].End))); j-- {
			ivs[j], ivs[j-1] = ivs[j-1], ivs[j]
		}
	}
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
