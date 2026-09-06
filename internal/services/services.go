// Copyright 2024 Jetify Inc. and contributors. All rights reserved.
// Use of this source code is governed by the license in the LICENSE file.

package services

type Services map[string]Service // name -> Service

type Service struct {
	Name               string
	ProcessComposePath string

	// Env is the static environment declared by the service's
	// process-compose file: the project-level environment overlaid with the
	// process's own. It is not the full environment the process runs with.
	Env map[string]string
}
