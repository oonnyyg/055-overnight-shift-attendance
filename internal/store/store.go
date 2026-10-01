package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"example.com/overnight-shift-attendance/internal/model"
	"example.com/overnight-shift-attendance/internal/timex"
)

const fileName = "attendance.json"

// Store wraps the model with validation, atomic imports and persistence.
type Store struct {
	data *model.Store
}

// Open loads a store from dir, starting empty when no file exists yet.
func Open(dir string) (*Store, error) {
	if dir == "" {
		return nil, errors.New("data directory must not be empty")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	s := &Store{data: model.NewStore()}
	path := filepath.Join(dir, fileName)
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read data file: %w", err)
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, s.data); err != nil {
			return nil, fmt.Errorf("decode data file: %w", err)
		}
	}
	if s.data.Employees == nil {
		s.data = model.NewStore()
	}
	return s, nil
}

// Save writes the store atomically.
func (s *Store) Save(dir string) error {
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, fileName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return fmt.Errorf("write data file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("commit data file: %w", err)
	}
	return nil
}

// Data returns the underlying data for read-only reporting.
func (s *Store) Data() *model.Store { return s.data }

// RegisterEmployee adds a single employee. Re-registering the same
// (id, name) is treated as a no-op; a changed name is a conflict.
func (s *Store) RegisterEmployee(e model.Employee) error {
	if e.ID == "" {
		return errors.New("employee id must not be empty")
	}
	if e.Name == "" {
		return errors.New("employee name must not be empty")
	}
	if existing, ok := s.data.Employees[e.ID]; ok {
		if existing.Name != e.Name {
			return fmt.Errorf("employee %q already exists with different name", e.ID)
		}
		return nil
	}
	s.data.Employees[e.ID] = e
	return nil
}

// ImportShifts validates and inserts shifts as one atomic batch.
func (s *Store) ImportShifts(in []model.Shift) (int, error) {
	staged := map[string]model.Shift{}
	added := 0
	for i, sh := range in {
		if err := validateShift(sh); err != nil {
			return 0, fmt.Errorf("shift[%d] %q: %w", i, sh.ID, err)
		}
		if _, ok := s.data.Employees[sh.EmpID]; !ok {
			return 0, fmt.Errorf("shift %q: unknown employee %q", sh.ID, sh.EmpID)
		}
		if existing, ok := s.data.Shifts[sh.ID]; ok {
			if !sameShift(existing, sh) {
				return 0, fmt.Errorf("shift id %q already exists with different content", sh.ID)
			}
			continue
		}
		if other, ok := staged[sh.ID]; ok {
			if !sameShift(other, sh) {
				return 0, fmt.Errorf("shift id %q repeated in batch with different content", sh.ID)
			}
			continue
		}
		sh.Breaks = timex.Merge(sh.Breaks)
		staged[sh.ID] = sh
		added++
	}
	for id, sh := range staged {
		if err := s.checkOverlap(sh, staged); err != nil {
			return 0, fmt.Errorf("shift %q: %w", id, err)
		}
	}
	for id, sh := range staged {
		s.data.Shifts[id] = sh
	}
	return added, nil
}

func validateShift(sh model.Shift) error {
	if sh.ID == "" {
		return errors.New("id must not be empty")
	}
	if sh.EmpID == "" {
		return errors.New("employee_id must not be empty")
	}
	if sh.Start.IsZero() || sh.End.IsZero() {
		return errors.New("start and end are required")
	}
	if sh.Start.Equal(sh.End) {
		return errors.New("start and end must not be identical")
	}
	if sh.End.Before(sh.Start) {
		return errors.New("end must not be before start")
	}
	span := timex.Interval{Start: sh.Start, End: sh.End}
	for _, br := range sh.Breaks {
		if !br.Valid() {
			return errors.New("break must have start before end")
		}
		if br.Start.Before(span.Start) || br.End.After(span.End) {
			return errors.New("break must lie within the shift")
		}
	}
	return nil
}

func (s *Store) checkOverlap(sh model.Shift, staged map[string]model.Shift) error {
	span := timex.Interval{Start: sh.Start, End: sh.End}
	for id, other := range s.data.Shifts {
		if id != sh.ID && other.EmpID == sh.EmpID && timex.Overlaps(span,
			timex.Interval{Start: other.Start, End: other.End}) {
			return fmt.Errorf("overlaps existing shift %q", id)
		}
	}
	for id, other := range staged {
		if id != sh.ID && other.EmpID == sh.EmpID && timex.Overlaps(span,
			timex.Interval{Start: other.Start, End: other.End}) {
			return fmt.Errorf("overlaps shift %q in same batch", id)
		}
	}
	return nil
}

func sameShift(a, b model.Shift) bool {
	if a.EmpID != b.EmpID || !a.Start.Equal(b.Start) || !a.End.Equal(b.End) ||
		len(a.Breaks) != len(b.Breaks) {
		return false
	}
	am := timex.Merge(a.Breaks)
	bm := timex.Merge(b.Breaks)
	for i := range am {
		if !am[i].Start.Equal(bm[i].Start) || !am[i].End.Equal(bm[i].End) {
			return false
		}
	}
	return true
}
