package attendance

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
)

const dataFileName = "data.json"

// Store 管理本地目录中的考勤数据。
type Store struct {
	dir  string
	data *Data
}

// Open 加载（或初始化）指定目录中的数据。
func Open(dir string) (*Store, error) {
	if dir == "" {
		return nil, errors.New("数据目录不能为空")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, data: newData()}
	raw, err := os.ReadFile(filepath.Join(dir, dataFileName))
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, s.data); err != nil {
		return nil, fmt.Errorf("数据文件损坏: %w", err)
	}
	return s, nil
}

// save 原子写回数据文件。
func (s *Store) save() error {
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(s.dir, dataFileName+".tmp")
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.dir, dataFileName))
}

// AddEmployee 登记员工。
func (s *Store) AddEmployee(e Employee) error {
	if e.ID == "" {
		return errors.New("员工标识不能为空")
	}
	if old, ok := s.data.Employees[e.ID]; ok {
		if reflect.DeepEqual(old, e) {
			return nil // 相同记录重复导入，忽略
		}
		return fmt.Errorf("员工标识 %q 冲突", e.ID)
	}
	s.data.Employees[e.ID] = e
	return s.save()
}

// shiftInterval 解析班次起止区间。
func shiftInterval(sh Shift) (Interval, error) {
	start, err := ParseTime(sh.Start)
	if err != nil {
		return Interval{}, err
	}
	end, err := ParseTime(sh.End)
	if err != nil {
		return Interval{}, err
	}
	if !end.After(start) {
		return Interval{}, fmt.Errorf("班次 %q 起止倒置或相同", sh.ID)
	}
	return Interval{Start: start, End: end}, nil
}

// shiftBreaks 解析并校验休息区间，返回并集。
func shiftBreaks(sh Shift, span Interval) ([]Interval, error) {
	var out []Interval
	for _, b := range sh.Breaks {
		start, err := ParseTime(b.Start)
		if err != nil {
			return nil, err
		}
		end, err := ParseTime(b.End)
		if err != nil {
			return nil, err
		}
		if !end.After(start) {
			return nil, fmt.Errorf("班次 %q 休息区间起止倒置或相同", sh.ID)
		}
		iv := Interval{Start: start, End: end}
		if iv.Start.Before(span.Start) || iv.End.After(span.End) {
			return nil, fmt.Errorf("班次 %q 休息区间超出班次范围", sh.ID)
		}
		out = append(out, iv)
	}
	return unionIntervals(out), nil
}

// ImportShifts 批量导入班次，全部校验通过才落库。
func (s *Store) ImportShifts(shifts []Shift) (int, error) {
	type parsed struct {
		sh   Shift
		span Interval
	}
	var toAdd []parsed
	seen := map[string]bool{}
	for _, sh := range shifts {
		if sh.ID == "" {
			return 0, errors.New("班次标识不能为空")
		}
		if seen[sh.ID] {
			return 0, fmt.Errorf("批次内班次标识 %q 重复", sh.ID)
		}
		seen[sh.ID] = true
		if _, ok := s.data.Employees[sh.EmployeeID]; !ok {
			return 0, fmt.Errorf("未知员工 %q", sh.EmployeeID)
		}
		span, err := shiftInterval(sh)
		if err != nil {
			return 0, err
		}
		if _, err := shiftBreaks(sh, span); err != nil {
			return 0, err
		}
		if old, ok := s.data.Shifts[sh.ID]; ok {
			if reflect.DeepEqual(old, sh) {
				continue // 相同记录重复导入，忽略
			}
			return 0, fmt.Errorf("班次标识 %q 冲突", sh.ID)
		}
		toAdd = append(toAdd, parsed{sh: sh, span: span})
	}
	// 同一员工班次不得重叠：与既有班次及本批次比较。
	for _, p := range toAdd {
		for _, old := range s.data.Shifts {
			if old.EmployeeID != p.sh.EmployeeID {
				continue
			}
			ospan, err := shiftInterval(old)
			if err != nil {
				continue
			}
			if overlaps(ospan, p.span) {
				return 0, fmt.Errorf("班次 %q 与既有班次 %q 重叠", p.sh.ID, old.ID)
			}
		}
		for _, q := range toAdd {
			if q.sh.ID != p.sh.ID && q.sh.EmployeeID == p.sh.EmployeeID && overlaps(q.span, p.span) {
				return 0, fmt.Errorf("班次 %q 与 %q 重叠", p.sh.ID, q.sh.ID)
			}
		}
	}
	for _, p := range toAdd {
		s.data.Shifts[p.sh.ID] = p.sh
	}
	if len(toAdd) > 0 {
		if err := s.save(); err != nil {
			return 0, err
		}
	}
	return len(toAdd), nil
}

// ImportLeaves 批量导入请假，全部校验通过才落库。
func (s *Store) ImportLeaves(leaves []Leave) (int, error) {
	var toAdd []Leave
	seen := map[string]bool{}
	for _, lv := range leaves {
		if lv.ID == "" {
			return 0, errors.New("请假标识不能为空")
		}
		if seen[lv.ID] {
			return 0, fmt.Errorf("批次内请假标识 %q 重复", lv.ID)
		}
		seen[lv.ID] = true
		if _, ok := s.data.Employees[lv.EmployeeID]; !ok {
			return 0, fmt.Errorf("未知员工 %q", lv.EmployeeID)
		}
		start, err := ParseTime(lv.Start)
		if err != nil {
			return 0, err
		}
		end, err := ParseTime(lv.End)
		if err != nil {
			return 0, err
		}
		if !end.After(start) {
			return 0, fmt.Errorf("请假 %q 起止倒置或相同", lv.ID)
		}
		if old, ok := s.data.Leaves[lv.ID]; ok {
			if reflect.DeepEqual(old, lv) {
				continue
			}
			return 0, fmt.Errorf("请假标识 %q 冲突", lv.ID)
		}
		toAdd = append(toAdd, lv)
	}
	for _, lv := range toAdd {
		s.data.Leaves[lv.ID] = lv
	}
	if len(toAdd) > 0 {
		if err := s.save(); err != nil {
			return 0, err
		}
	}
	return len(toAdd), nil
}

// ImportPunches 批量导入打卡，全部校验通过才落库。
func (s *Store) ImportPunches(punches []Punch) (int, error) {
	var toAdd []Punch
	seen := map[string]bool{}
	for _, p := range punches {
		if p.ID == "" {
			return 0, errors.New("打卡标识不能为空")
		}
		if seen[p.ID] {
			return 0, fmt.Errorf("批次内打卡标识 %q 重复", p.ID)
		}
		seen[p.ID] = true
		if _, ok := s.data.Employees[p.EmployeeID]; !ok {
			return 0, fmt.Errorf("未知员工 %q", p.EmployeeID)
		}
		sh, ok := s.data.Shifts[p.ShiftID]
		if !ok {
			return 0, fmt.Errorf("未知班次 %q", p.ShiftID)
		}
		if sh.EmployeeID != p.EmployeeID {
			return 0, fmt.Errorf("打卡 %q 员工与班次 %q 不一致", p.ID, p.ShiftID)
		}
		if p.Type != PunchIn && p.Type != PunchOut {
			return 0, fmt.Errorf("打卡 %q 类型非法", p.ID)
		}
		if _, err := ParseTime(p.Time); err != nil {
			return 0, err
		}
		if old, ok := s.data.Punches[p.ID]; ok {
			if reflect.DeepEqual(old, p) {
				continue
			}
			return 0, fmt.Errorf("打卡标识 %q 冲突", p.ID)
		}
		toAdd = append(toAdd, p)
	}
	for _, p := range toAdd {
		s.data.Punches[p.ID] = p
	}
	if len(toAdd) > 0 {
		if err := s.save(); err != nil {
			return 0, err
		}
	}
	return len(toAdd), nil
}
