package attendance

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"cron-job-azure/internal/logger"
)

// Item adalah payload absen yang dikirim ke API,
// field JSON-nya persis seperti objek pada src/services/AttendaceService.js.
type Item struct {
	MachineCode string `json:"MACHINE_CODE"`
	CompanyID   int    `json:"COMPANY_ID"`
	EmployeeNo  any    `json:"EMPLOYEE_NO"`
	InOutStatus string `json:"IN_OUT_STATUS"`
	AttendHours string `json:"ATTEND_HOURS"`
	AttendDate  string `json:"ATTEND_DATE"`
	Datetime    string `json:"DATETIME"`
	TempID      any    `json:"TEMP_ID"`
}

// GetAttendanceData adalah pengganti getAttendanceData() (database HKV).
func GetAttendanceData(ctx context.Context, pool *sql.DB, log *logger.Logger) ([]Item, []any, error) {
	const query = "select top 400 id, FORMAT([DateTime],'yyyy-MM-dd HH:mm') as DateTime," +
		"FORMAT([DateTime], 'yyyy-MM-dd') as date," +
		"FORMAT([DateTime], 'HH:mm') as time," +
		"[Date], [Time], seq_id  from  attendance where isnull(status,0) =0 order by [DateTime] asc"

	rows, err := pool.QueryContext(ctx, query)
	if err != nil {
		log.Errorf("Error fetching attendance data: %v", err)
		return nil, nil, err
	}
	defer rows.Close()

	data := []Item{}
	resultId := []any{}
	for rows.Next() {
		var id any
		var dateTime, date, timeStr string
		var dateCol, timeCol any
		var seqID any
		if err := rows.Scan(&id, &dateTime, &date, &timeStr, &dateCol, &timeCol, &seqID); err != nil {
			log.Errorf("Error fetching attendance data: %v", err)
			return nil, nil, err
		}
		data = append(data, Item{
			MachineCode: "HKV",
			CompanyID:   35077,
			EmployeeNo:  id,
			InOutStatus: "0",
			AttendHours: timeStr,
			AttendDate:  date,
			Datetime:    dateTime,
			TempID:      seqID,
		})
		resultId = append(resultId, seqID)
	}
	if err := rows.Err(); err != nil {
		log.Errorf("Error fetching attendance data: %v", err)
		return nil, nil, err
	}
	return data, resultId, nil
}

// GetAttendanceDataGD adalah pengganti getAttendanceDataGD() (database GD).
func GetAttendanceDataGD(ctx context.Context, pool *sql.DB, log *logger.Logger) ([]Item, []any, error) {
	const query = "select top 100 EMPNO,ATTENDDATA,DATE_FORMAT,FORMAT(TIME_FORMAT,'yyyy-MM-dd HH:mm') as TIME_FORMAT,FORMAT(TIME_FORMAT, 'HH:mm') as time," +
		"FORMAT(TIME_FORMAT, 'yyyy-MM-dd') as date from THRMATTENDANCETEMP where MACHINENO ='ATTGO' " +
		"and isnull(proc_sf7,0)=0 and TIME_FORMAT >= '2025-10-01' order by TIME_FORMAT asc"

	rows, err := pool.QueryContext(ctx, query)
	if err != nil {
		log.Errorf("Error fetching attendance data: %v", err)
		return nil, nil, err
	}
	defer rows.Close()

	data := []Item{}
	resultId := []any{}
	for rows.Next() {
		var empNo, attendData, dateFormat any
		var timeFormat, timeStr, date string
		if err := rows.Scan(&empNo, &attendData, &dateFormat, &timeFormat, &timeStr, &date); err != nil {
			log.Errorf("Error fetching attendance data: %v", err)
			return nil, nil, err
		}
		data = append(data, Item{
			MachineCode: "GD",
			CompanyID:   35077,
			EmployeeNo:  empNo,
			InOutStatus: "0",
			AttendHours: timeStr,
			AttendDate:  date,
			Datetime:    timeFormat,
			TempID:      attendData,
		})
		resultId = append(resultId, attendData)
	}
	if err := rows.Err(); err != nil {
		log.Errorf("Error fetching attendance data: %v", err)
		return nil, nil, err
	}
	return data, resultId, nil
}

// formatIDs mengubah daftar id (dari response API / database) menjadi
// potongan IN-clause, sejajar arraySecId.join(",") pada versi Node.js.
func formatIDs(ids []any, quote bool) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		switch v := id.(type) {
		case nil:
			continue
		case string:
			s := strings.ReplaceAll(v, "'", "''")
			if quote {
				s = "'" + s + "'"
			}
			parts = append(parts, s)
		case float64: // angka dari JSON response API
			parts = append(parts, strconv.FormatInt(int64(v), 10))
		case float32:
			parts = append(parts, strconv.FormatInt(int64(v), 10))
		case int64:
			parts = append(parts, strconv.FormatInt(v, 10))
		case int:
			parts = append(parts, strconv.Itoa(v))
		case json.Number:
			parts = append(parts, v.String())
		default:
			parts = append(parts, fmt.Sprint(v))
		}
	}
	return strings.Join(parts, ",")
}

// UpdateAttendanceStatus adalah pengganti updateAttendanceStatus()
// (update attendance set status = 1 where seq_id in (...)).
func UpdateAttendanceStatus(ctx context.Context, pool *sql.DB, ids []any, log *logger.Logger) error {
	if len(ids) > 0 {
		query := `update attendance set status = 1 where seq_id in (` + formatIDs(ids, false) + `)`
		if _, err := pool.ExecContext(ctx, query); err != nil {
			log.Errorf("Error updating attendance status: %v", err)
			return err
		}
	}
	return nil
}

// UpdateAttendanceStatusGD adalah pengganti updateAttendanceStatusGD()
// (update THRMATTENDANCETEMP set proc_sf7 = 1 where ATTENDDATA in ('...')).
// Sama seperti versi Node.js: dijalankan per nilai id.
func UpdateAttendanceStatusGD(ctx context.Context, pool *sql.DB, ids []any, log *logger.Logger) error {
	for _, id := range ids {
		query := `update THRMATTENDANCETEMP set proc_sf7 = 1 where ATTENDDATA in ('` +
			strings.ReplaceAll(fmt.Sprint(id), "'", "''") + `')`
		if _, err := pool.ExecContext(ctx, query); err != nil {
			log.Errorf("Error updating attendance status: %v", err)
			return err
		}
	}
	return nil
}
