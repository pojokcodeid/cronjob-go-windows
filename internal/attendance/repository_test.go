package attendance

import (
	"encoding/json"
	"testing"
)

func TestFormatIDsNumeric(t *testing.T) {
	// Hasil JSON API: angka -> float64 (sama seperti join(",") di Node.js).
	got := formatIDs([]any{float64(11), float64(12), float64(13)}, false)
	if got != "11,12,13" {
		t.Fatalf("formatIDs = %q, ingin 11,12,13", got)
	}
}

func TestFormatIDsStringQuoted(t *testing.T) {
	// GD memakai nilai string dengan kutip (seperti `in ('${value}')`).
	got := formatIDs([]any{"A001", "A002"}, true)
	if got != "'A001','A002'" {
		t.Fatalf("formatIDs = %q", got)
	}
}

func TestFormatIDsEscapeQuote(t *testing.T) {
	got := formatIDs([]any{"O'Brien"}, true)
	if got != "'O''Brien'" {
		t.Fatalf("formatIDs = %q", got)
	}
}

func TestFormatIDsCampuran(t *testing.T) {
	got := formatIDs([]any{float64(1), "2", nil, int64(3)}, false)
	if got != "1,2,3" {
		t.Fatalf("formatIDs = %q, ingin 1,2,3", got)
	}
}

func TestItemJSONKeySamaDenganNode(t *testing.T) {
	item := Item{
		MachineCode: "HKV",
		CompanyID:   35077,
		EmployeeNo:  float64(123),
		InOutStatus: "0",
		AttendHours: "08:15",
		AttendDate:  "2026-10-23",
		Datetime:    "2026-10-23 08:15",
		TempID:      float64(9),
	}
	b, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	// Key payload harus persis seperti objek pada versi Node.js.
	want := `{"MACHINE_CODE":"HKV","COMPANY_ID":35077,"EMPLOYEE_NO":123,` +
		`"IN_OUT_STATUS":"0","ATTEND_HOURS":"08:15","ATTEND_DATE":"2026-10-23",` +
		`"DATETIME":"2026-10-23 08:15","TEMP_ID":9}`
	if string(b) != want {
		t.Fatalf("payload JSON:\n got: %s\nwant: %s", b, want)
	}
}
