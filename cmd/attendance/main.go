// attendance 是跨夜班次本地考勤核算工具，逐行接收并返回 JSON。
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"example.com/overnight-shift-attendance/internal/attendance"
)

type request struct {
	Cmd      string             `json:"cmd"`
	ID       string             `json:"id"`
	Name     string             `json:"name"`
	Employee string             `json:"employee"`
	Shift    string             `json:"shift"`
	From     string             `json:"from"`
	To       string             `json:"to"`
	Shifts   []attendance.Shift `json:"shifts"`
	Leaves   []attendance.Leave `json:"leaves"`
	Punches  []attendance.Punch `json:"punches"`
}

func write(v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		raw = []byte(`{"ok":false,"error":"内部编码错误"}`)
	}
	fmt.Println(string(raw))
}

func fail(err error) {
	write(map[string]any{"ok": false, "error": err.Error()})
}

func handle(store *attendance.Store, req request) {
	switch req.Cmd {
	case "add_employee":
		if err := store.AddEmployee(attendance.Employee{ID: req.ID, Name: req.Name}); err != nil {
			fail(err)
			return
		}
		write(map[string]any{"ok": true})
	case "import_shifts":
		n, err := store.ImportShifts(req.Shifts)
		if err != nil {
			fail(err)
			return
		}
		write(map[string]any{"ok": true, "imported": n})
	case "import_leaves":
		n, err := store.ImportLeaves(req.Leaves)
		if err != nil {
			fail(err)
			return
		}
		write(map[string]any{"ok": true, "imported": n})
	case "import_punches":
		n, err := store.ImportPunches(req.Punches)
		if err != nil {
			fail(err)
			return
		}
		write(map[string]any{"ok": true, "imported": n})
	case "report":
		rep, err := store.Report(req.Employee, req.From, req.To)
		if err != nil {
			fail(err)
			return
		}
		write(map[string]any{"ok": true, "report": rep})
	case "shift_detail":
		detail, err := store.ShiftDetail(req.Shift)
		if err != nil {
			fail(err)
			return
		}
		write(map[string]any{"ok": true, "detail": detail})
	default:
		fail(fmt.Errorf("未知命令 %q", req.Cmd))
	}
}

func main() {
	dir := flag.String("dir", "", "本地数据目录（必填）")
	flag.Parse()
	if *dir == "" {
		fmt.Fprintln(os.Stderr, "缺少 -dir 参数")
		os.Exit(2)
	}
	store, err := attendance.Open(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 1024*1024), 16*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			fail(fmt.Errorf("非法 JSON 行: %w", err))
			continue
		}
		handle(store, req)
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
