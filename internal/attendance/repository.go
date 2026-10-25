package attendance

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	mssql "github.com/microsoft/go-mssqldb"

	"cron-job-azure/internal/logger"
)

const (
	batchHKV = 400
	batchGD  = 100

	// Batas driver SQL Server: 2100 parameter per statement.
	chunkHKV = 2000
	chunkGD  = 1000

	maxRetries = 3
	baseDelay  = 500 * time.Millisecond
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

// ---------------------------------------------------------------------------
// Helper: retry untuk error transient Azure SQL
// ---------------------------------------------------------------------------

func isTransient(err error) bool {
	var sqlErr mssql.Error
	if !errors.As(err, &sqlErr) {
		return false
	}
	switch sqlErr.Number {
	case 40613, 40501, 49918, 10928, 10929, 40197, 40540, 40143, 1205:
		return true
	}
	return false
}

// withRetry mengulang fn hanya untuk error transient, dengan exponential backoff.
// Aman dipakai untuk operasi idempoten (UPDATE ... AND status = 0).
func withRetry(ctx context.Context, log *logger.Logger, op string, fn func() error) error {
	var err error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if err = fn(); err == nil {
			return nil
		}
		if !isTransient(err) || ctx.Err() != nil {
			return err
		}
		delay := baseDelay << attempt
		log.Errorf("%s: error transient (percobaan %d/%d), ulang dalam %v: %v",
			op, attempt+1, maxRetries, delay, err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return err
}

// placeholders menghasilkan "@p1,@p2,...,@pN".
func placeholders(n int) string {
	var sb strings.Builder
	sb.Grow(n * 6)
	for i := 1; i <= n; i++ {
		if i > 1 {
			sb.WriteByte(',')
		}
		sb.WriteString("@p")
		sb.WriteString(strconv.Itoa(i))
	}
	return sb.String()
}

// execInChunks menjalankan: prefix + "(" + placeholders + ")" + suffix
// per chunk. conv (opsional) mengubah tiap id menjadi argumen bertipe tepat.
func execInChunks(
	ctx context.Context,
	pool *sql.DB,
	log *logger.Logger,
	op string,
	ids []any,
	size int,
	prefix, suffix string,
	conv func(any) any,
) error {
	for start := 0; start < len(ids); start += size {
		end := min(start+size, len(ids)) // Go 1.21+
		chunk := ids[start:end]

		args := make([]any, len(chunk))
		for i, id := range chunk {
			if conv != nil {
				args[i] = conv(id)
			} else {
				args[i] = id
			}
		}
		query := prefix + "(" + placeholders(len(chunk)) + ")" + suffix

		// var affected int64
		err := withRetry(ctx, log, op, func() error {
			_, err := pool.ExecContext(ctx, query, args...)
			if err != nil {
				return err
			}
			/* if n, e := res.RowsAffected(); e == nil {
				affected = n
			} */
			return nil
		})
		if err != nil {
			log.Errorf("%s: %v", op, err)
			return err
		}
		/* 	if affected != int64(len(chunk)) {
			log.Errorf("%s: diharapkan %d baris ter-update, hasil %d (kemungkinan sudah diproses sebelumnya)",
				op, len(chunk), affected)
		} */
	}
	return nil
}

// ---------------------------------------------------------------------------
// Database HKV
// ---------------------------------------------------------------------------

// GetAttendanceData adalah pengganti getAttendanceData() (database HKV).
// Memerlukan filtered index IX_Attendance_Pending (lihat attendance_indexes.sql).
func GetAttendanceData(ctx context.Context, pool *sql.DB, log *logger.Logger) ([]Item, []any, error) {
	const query = `
		SELECT TOP (400) a.ID, a.[DateTime], a.seq_id
		FROM attendance a
		WHERE a.status = 0
		ORDER BY a.[DateTime], a.seq_id`

	rows, err := pool.QueryContext(ctx, query)
	if err != nil {
		log.Errorf("Error fetching attendance data: %v", err)
		return nil, nil, err
	}
	defer rows.Close()

	data := make([]Item, 0, batchHKV)
	resultID := make([]any, 0, batchHKV)
	for rows.Next() {
		var id, seqID any
		var dt time.Time
		if err := rows.Scan(&id, &dt, &seqID); err != nil {
			log.Errorf("Error scanning attendance data: %v", err)
			return nil, nil, err
		}
		data = append(data, Item{
			MachineCode: "HKV",
			CompanyID:   35077,
			EmployeeNo:  id,
			InOutStatus: "0",
			AttendHours: dt.Format("15:04"),
			AttendDate:  dt.Format("2006-01-02"),
			Datetime:    dt.Format("2006-01-02 15:04"),
			TempID:      seqID,
		})
		resultID = append(resultID, seqID)
	}
	if err := rows.Err(); err != nil {
		log.Errorf("Error iterating attendance data: %v", err)
		return nil, nil, err
	}
	return data, resultID, nil
}

// UpdateAttendanceStatus adalah pengganti updateAttendanceStatus().
// Parameterized, idempoten (AND status = 0), dengan retry error transient.
func UpdateAttendanceStatus(ctx context.Context, pool *sql.DB, ids []any, log *logger.Logger) error {
	if len(ids) == 0 {
		return nil
	}
	return execInChunks(ctx, pool, log, "UpdateAttendanceStatus", ids, chunkHKV,
		"UPDATE attendance SET status = 1 WHERE seq_id IN ",
		" AND status = 0",
		nil,
	)
}

// ---------------------------------------------------------------------------
// Database GD
// ---------------------------------------------------------------------------

// GetAttendanceDataGD adalah pengganti getAttendanceDataGD() (database GD).
func GetAttendanceDataGD(ctx context.Context, pool *sql.DB, log *logger.Logger) ([]Item, []any, error) {
	const query = `
		SELECT TOP (100) EMPNO, ATTENDDATA, TIME_FORMAT
		FROM THRMATTENDANCETEMP
		WHERE MACHINENO = 'ATTGO'
		  AND ISNULL(proc_sf7, 0) = 0
		  AND TIME_FORMAT >= '2025-10-01'
		ORDER BY TIME_FORMAT ASC`

	rows, err := pool.QueryContext(ctx, query)
	if err != nil {
		log.Errorf("Error fetching attendance data GD: %v", err)
		return nil, nil, err
	}
	defer rows.Close()

	data := make([]Item, 0, batchGD)
	resultID := make([]any, 0, batchGD)
	for rows.Next() {
		var empNo, attendData any
		var tf time.Time
		if err := rows.Scan(&empNo, &attendData, &tf); err != nil {
			log.Errorf("Error scanning attendance data GD: %v", err)
			return nil, nil, err
		}
		data = append(data, Item{
			MachineCode: "GD",
			CompanyID:   35077,
			EmployeeNo:  empNo,
			InOutStatus: "0",
			AttendHours: tf.Format("15:04"),
			AttendDate:  tf.Format("2006-01-02"),
			Datetime:    tf.Format("2006-01-02 15:04"),
			TempID:      attendData,
		})
		resultID = append(resultID, attendData)
	}
	if err := rows.Err(); err != nil {
		log.Errorf("Error iterating attendance data GD: %v", err)
		return nil, nil, err
	}
	return data, resultID, nil
}

// UpdateAttendanceStatusGD adalah pengganti updateAttendanceStatusGD().
// Satu statement per chunk (bukan satu UPDATE per id).
//
// mssql.VarChar dipakai agar parameter bertipe VARCHAR (sama dengan kolom),
// sehingga tidak terjadi implicit conversion yang mematikan index.
// Jika kolom ATTENDDATA bertipe NVARCHAR, ganti conv menjadi nil-return string biasa.
func UpdateAttendanceStatusGD(ctx context.Context, pool *sql.DB, ids []any, log *logger.Logger) error {
	if len(ids) == 0 {
		return nil
	}
	return execInChunks(ctx, pool, log, "UpdateAttendanceStatusGD", ids, chunkGD,
		"UPDATE THRMATTENDANCETEMP SET proc_sf7 = 1 WHERE ATTENDDATA IN ",
		" AND ISNULL(proc_sf7, 0) = 0",
		func(id any) any { return mssql.VarChar(fmt.Sprint(id)) },
	)
}
