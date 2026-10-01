package attendance

import "sort"

// unionIntervals 合并重叠或相邻的区间，返回按开始时间排序的并集。
func unionIntervals(ivs []Interval) []Interval {
	sorted := append([]Interval(nil), ivs...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Start.Before(sorted[j].Start)
	})
	var out []Interval
	for _, iv := range sorted {
		if iv.Minutes() == 0 && !iv.End.After(iv.Start) {
			continue
		}
		if n := len(out); n > 0 && !iv.Start.After(out[n-1].End) {
			if iv.End.After(out[n-1].End) {
				out[n-1].End = iv.End
			}
			continue
		}
		out = append(out, iv)
	}
	return out
}

// subtractIntervals 从 base 的并集中去掉 cuts 的并集。
func subtractIntervals(base, cuts []Interval) []Interval {
	result := unionIntervals(base)
	for _, c := range unionIntervals(cuts) {
		var next []Interval
		for _, b := range result {
			if !c.Start.Before(b.End) || !c.End.After(b.Start) {
				next = append(next, b)
				continue
			}
			if c.Start.After(b.Start) {
				next = append(next, Interval{Start: b.Start, End: c.Start})
			}
			if c.End.Before(b.End) {
				next = append(next, Interval{Start: c.End, End: b.End})
			}
		}
		result = next
	}
	return result
}

// intersectIntervals 返回两组区间（各自视为并集）的交集。
func intersectIntervals(a, b []Interval) []Interval {
	ua, ub := unionIntervals(a), unionIntervals(b)
	var out []Interval
	i, j := 0, 0
	for i < len(ua) && j < len(ub) {
		start := ua[i].Start
		if ub[j].Start.After(start) {
			start = ub[j].Start
		}
		end := ua[i].End
		if ub[j].End.Before(end) {
			end = ub[j].End
		}
		if start.Before(end) {
			out = append(out, Interval{Start: start, End: end})
		}
		if ua[i].End.Before(ub[j].End) {
			i++
		} else {
			j++
		}
	}
	return out
}

// totalMinutes 返回区间列表的总分钟数。
func totalMinutes(ivs []Interval) int {
	total := 0
	for _, iv := range ivs {
		total += iv.Minutes()
	}
	return total
}

// overlaps 判断两个左闭右开区间是否重叠。
func overlaps(a, b Interval) bool {
	return a.Start.Before(b.End) && b.Start.Before(a.End)
}
