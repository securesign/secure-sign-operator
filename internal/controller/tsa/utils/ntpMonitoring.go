package tsaUtils

type NtpConfig struct {
	RequestAttempts int32    `yaml:"request_attempts"`
	RequestTimeout  int32    `yaml:"request_timeout"`
	NumServers      int32    `yaml:"num_servers"`
	MaxTimeDelta    int32    `yaml:"max_time_delta"`
	ServerThreshold int32    `yaml:"server_threshold"`
	Period          int32    `yaml:"period"`
	Servers         []string `yaml:"servers"`
}
