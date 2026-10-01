// Command attendance is a line-oriented JSON attendance service.
//
// Each stdin line must be one JSON request; each stdout line is one JSON
// response. Errors are reported per line and never stop the stream.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"example.com/overnight-shift-attendance/internal/app"
)

type envelope struct {
	Op      string          `json:"op"`
	Payload json.RawMessage `json:"payload"`
}

type okResponse struct {
	OK       bool        `json:"ok"`
	Added    int         `json:"added,omitempty"`
	Shifts   interface{} `json:"shifts,omitempty"`
	Employee interface{} `json:"employees,omitempty"`
}

type errResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

func main() {
	dir := flag.String("dir", "./data", "directory for the local attendance data file")
	flag.Parse()

	svc, err := app.New(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	in := bufio.NewReader(os.Stdin)
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()

	for {
		line, readErr := in.ReadBytes('\n')
		if len(line) > 0 {
			writeResponse(out, dispatch(svc, line))
		}
		if readErr != nil {
			return
		}
	}
}

func dispatch(svc *app.App, line []byte) interface{} {
	var req envelope
	if err := json.Unmarshal(line, &req); err != nil {
		return errResponse{Error: "invalid JSON: " + err.Error()}
	}
	switch req.Op {
	case "register_employee":
		var p app.EmployeeInput
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse{Error: "invalid payload: " + err.Error()}
		}
		if err := svc.RegisterEmployee(p); err != nil {
			return errResponse{Error: err.Error()}
		}
		return okResponse{OK: true}

	case "import_shifts":
		var p []app.ShiftInput
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse{Error: "invalid payload: " + err.Error()}
		}
		added, err := svc.ImportShifts(p)
		if err != nil {
			return errResponse{Error: err.Error()}
		}
		return okResponse{OK: true, Added: added}

	case "import_leaves":
		var p []app.LeaveInput
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse{Error: "invalid payload: " + err.Error()}
		}
		added, err := svc.ImportLeaves(p)
		if err != nil {
			return errResponse{Error: err.Error()}
		}
		return okResponse{OK: true, Added: added}

	case "import_punches":
		var p []app.PunchInput
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse{Error: "invalid payload: " + err.Error()}
		}
		added, err := svc.ImportPunches(p)
		if err != nil {
			return errResponse{Error: err.Error()}
		}
		return okResponse{OK: true, Added: added}

	case "report", "detail":
		var p app.ReportInput
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return errResponse{Error: "invalid payload: " + err.Error()}
		}
		report, err := svc.Report(p)
		if err != nil {
			return errResponse{Error: err.Error()}
		}
		return okResponse{OK: true, Shifts: report.Shifts, Employee: report.Employee}

	default:
		return errResponse{Error: "unknown op: " + req.Op}
	}
}

func writeResponse(out *bufio.Writer, v interface{}) {
	raw, err := json.Marshal(v)
	if err != nil {
		raw = []byte(`{"ok":false,"error":"internal marshal failure"}`)
	}
	out.Write(raw)
	out.WriteByte('\n')
}
