package helm

import (
	"context"
	"os"
)

func DefaultDriver() string {
	if os.Getenv(driverEnv) == DriverConfigMap {
		return DriverConfigMap
	}
	return DriverSecret
}

func (s *Service) ReleaseDriver(ctx context.Context, namespace, name string) string {
	if s == nil {
		return DefaultDriver()
	}
	if s.cs == nil {
		return DefaultDriver()
	}
	if !nameFormat.MatchString(namespace) {
		return DefaultDriver()
	}
	if !nameFormat.MatchString(name) {
		return DefaultDriver()
	}
	found, err := s.driverFor(ctx, namespace, name)
	if err != nil {
		return DefaultDriver()
	}
	return found
}
