// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv

import (
	"runtime"
	"strings"

	"github.com/juju/utils/featureflag"
)

const (
	JujuModelEnvKey         = "JUJU_MODEL"
	JujuHomeEnvKey          = "JUJU_HOME"
	JujuRepositoryEnvKey    = "JUJU_REPOSITORY"
	JujuLoggingConfigEnvKey = "JUJU_LOGGING_CONFIG"
	JujuFeatureFlagEnvKey   = "JUJU_DEV_FEATURE_FLAGS"

	// JujuStartupLoggingConfigEnvKey if set is used to configure the initial
	// logging before the command objects are even created to allow debugging
	// of the command creation and initialisation process.
	JujuStartupLoggingConfigEnvKey = "JUJU_STARTUP_LOGGING_CONFIG"

	// Registry key containing juju related information
	JujuRegistryKey = `HKLM:\SOFTWARE\juju-core`

	// Registry value where the jujud password resides
	JujuRegistryPasswordKey = `jujud-password`

	// TODO(thumper): 2013-09-02 bug 1219630
	// As much as I'd like to remove JujuContainerType now, it is still
	// needed as MAAS still needs it at this stage, and we can't fix
	// everything at once.
	JujuContainerTypeEnvKey = "JUJU_CONTAINER_TYPE"

	// JujuStatusIsoTimeEnvKey is the env var which if true, will cause status
	// timestamps to be written in RFC3339 format.
	JujuStatusIsoTimeEnvKey = "JUJU_STATUS_ISO_TIME"
)

// FeatureFlags returns a map that can be merged with os.Environ.
func FeatureFlags() map[string]string {
	result := make(map[string]string)
	if envVar := featureflag.AsEnvironmentValue(); envVar != "" {
		result[JujuFeatureFlagEnvKey] = envVar
	}
	return result
}

// MergeEnvironment will return the current environment updated with
// all the values from newValues.  If current is nil, a new map is
// created.  If current is not nil, it is mutated.
func MergeEnvironment(current, newValues map[string]string) map[string]string {
	if current == nil {
		current = make(map[string]string)
	}
	if runtime.GOOS == "windows" {
		return mergeEnvWin(current, newValues)
	}
	return mergeEnvUnix(current, newValues)
}

// mergeEnvUnix merges the two evironment variable lists in a case sensitive way.
func mergeEnvUnix(current, newValues map[string]string) map[string]string {
	for key, value := range newValues {
		current[key] = value
	}
	return current
}

// mergeEnvWin merges the two environment variable lists in a case insensitive,
// but case preserving way.  Thus, if FOO=bar is set, and newValues has foo=baz,
// then the resultant map will contain FOO=baz.
func mergeEnvWin(current, newValues map[string]string) map[string]string {
	uppers := make(map[string]string, len(current))
	news := map[string]string{}
	for k, v := range current {
		uppers[strings.ToUpper(k)] = v
	}

	for k, v := range newValues {
		up := strings.ToUpper(k)
		if _, ok := uppers[up]; ok {
			uppers[up] = v
		} else {
			news[k] = v
		}
	}

	for k := range current {
		current[k] = uppers[strings.ToUpper(k)]
	}
	for k, v := range news {
		current[k] = v
	}
	return current
}
