package autofree

import "time"

type Config struct {
	SupportedEnvs []string      `yaml:"supportedEnvs"`
	AccountIDP    string        `yaml:"accountIdp"`
	Account       string        `yaml:"account"`
	JobInterval   time.Duration `yaml:"jobInterval"`
	BatchInterval time.Duration `yaml:"batchInterval"`
	BatchSize     int           `yaml:"batchSize"`
}
