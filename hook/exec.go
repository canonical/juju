// The hook package provides a mechanism by which charm hooks can be executed in
// appropriate environments.
package hook

import (
	"os"
	"os/exec"
	"path/filepath"
)

// ExecContext exposes the parts of Context that are necessary to integrate
// with Exec. This is useful partly because Context is not yet implemented, and
// partly for ease of testing.
type ExecContext interface {
	Vars() []string
	Flush() error
}

// ExecInfo is responsible for constructing those parts of a hook execution
// environment which cannot be inferred from the Context itself.
type ExecInfo struct {
	ContextId   string
	AgentSocket string
	CharmDir    string
	RemoteUnit  string
}

// Vars returns an os.Environ-style list of strings.
func (info *ExecInfo) Vars() []string {
	vars := []string{
		"APT_LISTCHANGES_FRONTEND=none",
		"DEBIAN_FRONTEND=noninteractive",
		"PATH=" + os.Getenv("PATH"),
		"CHARM_DIR=" + info.CharmDir,
		"JUJU_CONTEXT_ID=" + info.ContextId,
		"JUJU_AGENT_SOCKET=" + info.AgentSocket,
	}
	if info.RemoteUnit != "" {
		vars = append(vars, "JUJU_REMOTE_UNIT="+info.RemoteUnit)
	}
	return vars
}

// isImportant returns false if err indicates that the hook didn't exist in the
// first place.
func isImportant(err error) bool {
	ee, _ := err.(*exec.Error)
	if ee == nil {
		return true
	}
	return !os.IsNotExist(ee.Err)
}

// Exec executes the named hook in the environment defined by ctx and info.
func Exec(hookName string, info *ExecInfo, ctx ExecContext) error {
	ps := exec.Command(filepath.Join(info.CharmDir, "hooks", hookName))
	ps.Dir = info.CharmDir
	ps.Env = append(info.Vars(), ctx.Vars()...)
	if err := ps.Run(); err != nil {
		if isImportant(err) {
			return err
		}
		return nil
	}
	return ctx.Flush()
}
