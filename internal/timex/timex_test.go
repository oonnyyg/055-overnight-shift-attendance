package timex

import "testing"

func TestParseFormatRoundTrip(t *testing.T) {
	tm, err := Parse("2026-10-01 08:00")
	if err != nil {
		t.Fatal(err)
	}
	if got := Format(tm); got != "2026-10-01 08:00" {
		t.Fatalf("round trip = %q", got)
	}
	if got := Date(tm); got != "2026-10-01" {
		t.Fatalf("date = %q", got)
	}
	if _, err := Parse("2026-10-1 08:00"); err == nil {
		t.Fatal("expected parse error for bad format")
	}
	if _, err := Parse("2026-13-01 08:00"); err == nil {
		t.Fatal("expected parse error for illegal month")
	}
}

func iv(t *testing.T, a, b string) Interval {
	t.Helper()
	sa, err := Parse(a)
	if err != nil {
		t.Fatal(err)
	}
	sb, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	return Interval{Start: sa, End: sb}
}

func TestIntervalOps(t *testing.T) {
	a := iv(t, "2026-10-01 22:00", "2026-10-02 06:00")
	if a.Minutes() != 8*60 {
		t.Fatalf("overnight minutes = %d", a.Minutes())
	}
	b := iv(t, "2026-10-02 02:00", "2026-10-02 03:00")
	if Intersect(a, b).Minutes() != 60 {
		t.Fatal("overnight intersect failed")
	}
	rest := Subtract(a, b)
	if SumMinutes(rest) != 7*60 {
		t.Fatalf("subtract minutes = %d", SumMinutes(rest))
	}
}

func TestMergeAndSubtractAll(t *testing.T) {
	a := iv(t, "2026-10-01 09:00", "2026-10-01 18:00")
	cuts := []Interval{
		iv(t, "2026-10-01 12:00", "2026-10-01 13:00"),
		iv(t, "2026-10-01 12:30", "2026-10-01 13:30"),
		iv(t, "2026-10-01 15:00", "2026-10-01 15:30"),
	}
	got := SubtractAll(a, cuts)
	if SumMinutes(got) != 9*60-120 {
		t.Fatalf("union-subtract minutes = %d", SumMinutes(got))
	}
	if len(Merge(cuts)) != 2 {
		t.Fatal("merge should collapse overlap and keep separate gap")
	}
}

func TestTouchingIntervals(t *testing.T) {
	a := iv(t, "2026-10-01 09:00", "2026-10-01 12:00")
	b := iv(t, "2026-10-01 12:00", "2026-10-01 14:00")
	if Overlaps(a, b) {
		t.Fatal("half-open touching intervals must not overlap")
	}
	if merged, ok := Union(a, b); !ok || merged.Minutes() != 5*60 {
		t.Fatal("touching intervals should union")
	}
}
