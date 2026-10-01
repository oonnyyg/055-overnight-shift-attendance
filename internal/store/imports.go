package store

import (
	"errors"
	"fmt"

	"example.com/overnight-shift-attendance/internal/model"
)

// ImportLeaves validates and inserts leaves as one atomic batch.
func (s *Store) ImportLeaves(in []model.Leave) (int, error) {
	staged := map[string]model.Leave{}
	added := 0
	for i, lv := range in {
		if err := validateLeave(lv); err != nil {
			return 0, fmt.Errorf("leave[%d] %q: %w", i, lv.ID, err)
		}
		if _, ok := s.data.Employees[lv.EmpID]; !ok {
			return 0, fmt.Errorf("leave %q: unknown employee %q", lv.ID, lv.EmpID)
		}
		if existing, ok := s.data.Leaves[lv.ID]; ok {
			if !sameLeave(existing, lv) {
				return 0, fmt.Errorf("leave id %q already exists with different content", lv.ID)
			}
			continue
		}
		if other, ok := staged[lv.ID]; ok {
			if !sameLeave(other, lv) {
				return 0, fmt.Errorf("leave id %q repeated in batch with different content", lv.ID)
			}
			continue
		}
		staged[lv.ID] = lv
		added++
	}
	for id, lv := range staged {
		s.data.Leaves[id] = lv
	}
	return added, nil
}

func validateLeave(lv model.Leave) error {
	if lv.ID == "" {
		return errors.New("id must not be empty")
	}
	if lv.EmpID == "" {
		return errors.New("employee_id must not be empty")
	}
	if lv.Start.IsZero() || lv.End.IsZero() {
		return errors.New("start and end are required")
	}
	if lv.End.Before(lv.Start) {
		return errors.New("end must not be before start")
	}
	if lv.Start.Equal(lv.End) {
		return errors.New("start and end must not be identical")
	}
	return nil
}

func sameLeave(a, b model.Leave) bool {
	return a.EmpID == b.EmpID && a.Start.Equal(b.Start) && a.End.Equal(b.End)
}

// ImportPunches validates and inserts punches as one atomic batch.
func (s *Store) ImportPunches(in []model.Punch) (int, error) {
	staged := map[string]model.Punch{}
	added := 0
	for i, p := range in {
		if err := validatePunch(p); err != nil {
			return 0, fmt.Errorf("punch[%d] %q: %w", i, p.ID, err)
		}
		if _, ok := s.data.Employees[p.EmpID]; !ok {
			return 0, fmt.Errorf("punch %q: unknown employee %q", p.ID, p.EmpID)
		}
		sh, ok := s.data.Shifts[p.ShiftID]
		if !ok {
			return 0, fmt.Errorf("punch %q: unknown shift %q", p.ID, p.ShiftID)
		}
		if sh.EmpID != p.EmpID {
			return 0, fmt.Errorf("punch %q: employee %q does not own shift %q", p.ID, p.EmpID, p.ShiftID)
		}
		if existing, ok := s.data.Punches[p.ID]; ok {
			if !samePunch(existing, p) {
				return 0, fmt.Errorf("punch id %q already exists with different content", p.ID)
			}
			continue
		}
		if other, ok := staged[p.ID]; ok {
			if !samePunch(other, p) {
				return 0, fmt.Errorf("punch id %q repeated in batch with different content", p.ID)
			}
			continue
		}
		staged[p.ID] = p
		added++
	}
	for id, p := range staged {
		s.data.Punches[id] = p
	}
	return added, nil
}

func validatePunch(p model.Punch) error {
	if p.ID == "" {
		return errors.New("id must not be empty")
	}
	if p.EmpID == "" {
		return errors.New("employee_id must not be empty")
	}
	if p.ShiftID == "" {
		return errors.New("shift_id must not be empty")
	}
	if p.Kind != model.KindIn && p.Kind != model.KindOut {
		return errors.New("kind must be \"in\" or \"out\"")
	}
	if p.At.IsZero() {
		return errors.New("at is required")
	}
	return nil
}

func samePunch(a, b model.Punch) bool {
	return a.EmpID == b.EmpID && a.ShiftID == b.ShiftID && a.Kind == b.Kind && a.At.Equal(b.At)
}
