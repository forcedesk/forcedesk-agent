package db

import "time"

// ProbeHistorySample is one row from probe_history, shaped for graph rendering.
type ProbeHistorySample struct {
	AvgMS      *float64
	MinMS      *float64
	MaxMS      *float64
	PacketLoss int
}

// SaveProbeHistory inserts one probe measurement and prunes rows older than 7 days
// for the same probe to keep the table from growing unbounded.
func SaveProbeHistory(probeID int64, ts time.Time, avg, min, max *float64, packetLoss int) error {
	_, err := DB.Exec(
		`INSERT INTO probe_history (probe_id, ts, avg_ms, min_ms, max_ms, packet_loss)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		probeID, ts.Unix(), avg, min, max, packetLoss,
	)
	if err != nil {
		return err
	}
	// Prune data older than 7 days for this probe.
	cutoff := ts.Add(-7 * 24 * time.Hour).Unix()
	_, err = DB.Exec(
		`DELETE FROM probe_history WHERE probe_id = ? AND ts < ?`,
		probeID, cutoff,
	)
	return err
}

// GetProbeHistory returns the most recent n measurements for a probe,
// returned oldest-first so they map left→right on a graph.
func GetProbeHistory(probeID int64, n int) ([]ProbeHistorySample, error) {
	rows, err := DB.Query(
		`SELECT avg_ms, min_ms, max_ms, packet_loss
		 FROM (
		   SELECT avg_ms, min_ms, max_ms, packet_loss, ts
		   FROM probe_history
		   WHERE probe_id = ?
		   ORDER BY ts DESC
		   LIMIT ?
		 )
		 ORDER BY ts ASC`,
		probeID, n,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ProbeHistorySample
	for rows.Next() {
		var s ProbeHistorySample
		if err := rows.Scan(&s.AvgMS, &s.MinMS, &s.MaxMS, &s.PacketLoss); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
