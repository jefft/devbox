// Copyright 2024 Jetify Inc. and contributors. All rights reserved.
// Use of this source code is governed by the license in the LICENSE file.

package plugin

import (
	"maps"

	"go.jetify.com/devbox/internal/boxcli/usererr"
	"go.jetify.com/devbox/internal/services"
)

// GetServices merges the services of every config, erroring if two distinct
// configs define the same service name.
func GetServices(configs []*Config) (services.Services, error) {
	allSvcs := services.Services{}
	origins := map[string]string{} // service name -> Source.LockfileKey
	for _, conf := range configs {
		svcs, err := conf.Services()
		if err != nil {
			return nil, usererr.New(
				"reading services in %q (from %s): %v",
				conf.Source.CanonicalName(), conf.Source.LockfileKey(), err)
		}
		for name := range svcs {
			if prev, ok := origins[name]; ok {
				return nil, usererr.New(
					"service %q is defined by two includes: %q and %q",
					name, prev, conf.Source.LockfileKey())
			}
			origins[name] = conf.Source.LockfileKey()
		}
		maps.Copy(allSvcs, svcs)
	}
	return allSvcs, nil
}
