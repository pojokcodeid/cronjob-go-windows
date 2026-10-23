package attendance

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"cron-job-azure/internal/api"
	"cron-job-azure/internal/logger"
)

// endpoint tujuan, sama seperti pada src/controllers/AttendacneSend.js.
const sendPath = "?qlid=CL_AttendanceTemp.setProcessAtt"

func logErrorDetails(log *logger.Logger, err error) {
	log.Errorf("Error process send attendance: %v", err)
	var httpErr *api.HTTPError
	if errors.As(err, &httpErr) {
		log.Errorf("Status: %d", httpErr.StatusCode)
		log.Errorf("Data dari server: %s", httpErr.Body)
	}
}

// SendHKV adalah pengganti AttendacneSend():
// ambil data HKV -> kirim ke API -> update status bila HSTATUS == 200.
func SendHKV(ctx context.Context, pool *sql.DB, client *api.Client, baseURL string, log *logger.Logger) (*api.Result, error) {
	resultData, _, err := GetAttendanceData(ctx, pool, log)
	if err != nil {
		return nil, err
	}
	log.Debugf("%s", mustJSON(resultData))

	resp, _, err := client.PostJSON(ctx, baseURL+sendPath, map[string]any{"data": resultData})
	if err != nil {
		logErrorDetails(log, err)
		return nil, err
	}
	log.Debugf("%s", mustJSON(resp.Data.RESULT))

	// Sama seperti versi Node.js: update memakai RESULT dari response API.
	if resp.HSTATUS.String() == "200" {
		if err := UpdateAttendanceStatus(ctx, pool, resp.Data.RESULT, log); err != nil {
			return resp, err
		}
		log.Infof("Send Attendance %d Success ...", len(resultData))
	}
	return resp, nil
}

// SendGD adalah pengganti AttendacneSendGD() untuk database GD.
func SendGD(ctx context.Context, pool *sql.DB, client *api.Client, baseURL string, log *logger.Logger) (*api.Result, error) {
	resultData, _, err := GetAttendanceDataGD(ctx, pool, log)
	if err != nil {
		return nil, err
	}
	log.Debugf("%s", mustJSON(resultData))

	resp, _, err := client.PostJSON(ctx, baseURL+sendPath, map[string]any{"data": resultData})
	if err != nil {
		logErrorDetails(log, err)
		return nil, err
	}

	if resp.HSTATUS.String() == "200" {
		if err := UpdateAttendanceStatusGD(ctx, pool, resp.Data.RESULT, log); err != nil {
			return resp, err
		}
		log.Infof("Send Attendance GD %d Success ...", len(resultData))
	}
	return resp, nil
}

func mustJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%+v", v)
	}
	return string(b)
}
