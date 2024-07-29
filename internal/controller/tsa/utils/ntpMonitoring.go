package tsaUtils

type NtpConfig struct {
	RequestAttempts int      `yaml:"request_attempts"`
	RequestTimeout  int      `yaml:"request_timeout"`
	NumServers      int      `yaml:"num_servers"`
	MaxTimeDelta    int      `yaml:"max_time_delta"`
	ServerThreshold int      `yaml:"server_threshold"`
	Period          int      `yaml:"period"`
	Servers         []string `yaml:"servers"`
}
