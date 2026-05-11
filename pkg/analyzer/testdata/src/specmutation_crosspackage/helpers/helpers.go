package helpers

type Config struct {
	Host string
	Port int
}

type Spec struct {
	Config *Config
}

type Status struct {
	Ready bool
}

type Instance struct {
	Spec   Spec
	Status Status
}

// MutateConfig writes through its pointer parameter.
func MutateConfig(cfg *Config) {
	cfg.Host = "mutated"
}

// ReadConfig only reads from the pointer parameter.
func ReadConfig(cfg *Config) string {
	return cfg.Host
}

// MutateSpecViaObject receives a whole object and mutates Spec.
func MutateSpecViaObject(obj *Instance) {
	obj.Spec.Config.Host = "mutated"
}

// MutateStatusViaObject receives a whole object but only mutates Status.
func MutateStatusViaObject(obj *Instance) {
	obj.Status.Ready = true
}
