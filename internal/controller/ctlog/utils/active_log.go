package utils

import rhtasv1 "github.com/securesign/operator/api/v1"

// ActiveLog returns a pointer to the active log entry from the Logs slice,
// or nil if no log has Active set to true.
func ActiveLog(logs []rhtasv1.CTLogConfig) *rhtasv1.CTLogConfig {
	for i := range logs {
		if logs[i].Active != nil && *logs[i].Active {
			return &logs[i]
		}
	}
	return nil
}

// ActiveLogPrefix returns the Prefix of the active log, or empty string if
// there is no active log.
func ActiveLogPrefix(logs []rhtasv1.CTLogConfig) string {
	if l := ActiveLog(logs); l != nil {
		return l.Prefix
	}
	return ""
}
