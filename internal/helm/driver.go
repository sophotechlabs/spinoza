package helm

import (
	"context"
	"os"
)

// DefaultDriver is where a release that does not exist yet will go.
func DefaultDriver() string {
	if os.Getenv(driverEnv) == DriverConfigMap {
		return DriverConfigMap
	}
	return DriverSecret
}

// ReleaseDriver is where the history is actually kept, which the environment
// can disagree with. A release nobody can find is answered for as a new one.
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
