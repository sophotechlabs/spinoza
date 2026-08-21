package helm

import (
	"context"
	"os"
)

// DefaultDriver is where helm would keep a release it has not been told about:
// the environment decides, and secrets are what it settles on. It is what a
// release that does not exist yet will be stored in.
func DefaultDriver() string {
	if os.Getenv(driverEnv) == DriverConfigMap {
		return DriverConfigMap
	}
	return DriverSecret
}

// ReleaseDriver is where this release's history is actually kept, which is the
// only authority on it: the environment can say one thing and the cluster hold
// another. A release nobody can find is answered for as a new one would be.
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
