// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	proxyutils "github.com/juju/proxy"

	k8sexec "github.com/juju/juju/caas/kubernetes/provider/exec"
	jujucmd "github.com/juju/juju/cmd"
	agentcmd "github.com/juju/juju/cmd/jujud/agent"
	"github.com/juju/juju/cmd/jujud/agent/caasoperator"
	"github.com/juju/juju/cmd/jujud/dumplogs"
	"github.com/juju/juju/cmd/jujud/hooktool"
	"github.com/juju/juju/cmd/jujud/introspect"
	"github.com/juju/juju/cmd/jujud/run"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	components "github.com/juju/juju/component/all"
	"github.com/juju/juju/core/machinelock"
	jujunames "github.com/juju/juju/juju/names"
	"github.com/juju/juju/upgrades"
	"github.com/juju/juju/utils/proxy"
	"github.com/juju/juju/worker/logsender"

	// Import the providers.
	_ "github.com/juju/juju/provider/all"
)

var logger = loggo.GetLogger("juju.cmd.jujud")

func init() {
	if err := components.RegisterForServer(); err != nil {
		logger.Criticalf("unable to register server components: %v", err)
		os.Exit(1)
	}
}

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

var jujudDoc = `
juju provides easy, intelligent service orchestration on top of models
such as OpenStack, Amazon AWS, or bare metal. jujud is a component of juju.

https://jujucharms.com/

The jujud command can also forward invocations over RPC for execution by the
juju unit agent. When used in this way, it expects to be called via a symlink
named for the desired remote command, and expects JUJU_AGENT_SOCKET_ADDRESS and
JUJU_CONTEXT_ID be set in its model.
`

const (
	// exit_err is the value that is returned when the user has run juju in an invalid way.
	exit_err = 2
	// exit_panic is the value that is returned when we exit due to an unhandled panic.
	exit_panic = 3
)

// Main registers subcommands for the jujud executable, and hands over control
// to the cmd package.
func jujuDMain(args []string, ctx *cmd.Context) (code int, err error) {
	// Assuming an average of 200 bytes per log message, use up to
	// 200MB for the log buffer.
	defer logger.Debugf("jujud complete, code %d, err %v", code, err)
	bufferedLogger, err := logsender.InstallBufferedLogWriter(loggo.DefaultContext(), 1048576)
	if err != nil {
		return 1, errors.Trace(err)
	}

	// Set the default transport to use the in-process proxy
	// configuration.
	if err := proxy.DefaultConfig.Set(proxyutils.DetectProxies()); err != nil {
		return 1, errors.Trace(err)
	}
	if err := proxy.DefaultConfig.InstallInDefaultTransport(); err != nil {
		return 1, errors.Trace(err)
	}

	jujud := jujucmd.NewSuperCommand(cmd.SuperCommandParams{
		Name: "jujud",
		Doc:  jujudDoc,
	})

	jujud.Log.NewWriter = func(target io.Writer) loggo.Writer {
		return &jujudWriter{target: target}
	}

	jujud.Register(agentcmd.NewBootstrapCommand())
	jujud.Register(agentcmd.NewCAASUnitInitCommand())
	jujud.Register(agentcmd.NewModelCommand())

	// TODO(katco-): AgentConf type is doing too much. The
	// MachineAgent type has called out the separate concerns; the
	// AgentConf should be split up to follow suit.
	agentConf := agentcmd.NewAgentConf("")
	machineAgentFactory := agentcmd.MachineAgentFactoryFn(
		agentConf,
		bufferedLogger,
		agentcmd.DefaultIntrospectionSocketName,
		upgrades.PreUpgradeSteps,
		"",
	)
	jujud.Register(agentcmd.NewMachineAgentCmd(ctx, machineAgentFactory, agentConf, agentConf))

	unitAgent, err := agentcmd.NewUnitAgent(ctx, bufferedLogger)
	if err != nil {
		return -1, errors.Trace(err)
	}
	jujud.Register(unitAgent)

	caasOperatorAgent, err := agentcmd.NewCaasOperatorAgent(ctx, bufferedLogger, func(mc *caasoperator.ManifoldsConfig) error {
		mc.NewExecClient = k8sexec.NewInCluster
		return nil
	})
	if err != nil {
		return -1, errors.Trace(err)
	}
	jujud.Register(caasOperatorAgent)

	jujud.Register(agentcmd.NewCheckConnectionCommand(agentConf, agentcmd.ConnectAsAgent))

	code = cmd.Main(jujud, ctx, args[1:])
	return code, nil
}

// MainWrapper exists to preserve test functionality.
// On windows we need to catch the return code from main for
// service functionality purposes, but on unix we can just os.Exit
func MainWrapper(args []string) {
	os.Exit(Main(args))
}

// Main is not redundant with main(), because it provides an entry point
// for testing with arbitrary command line arguments.
func Main(args []string) int {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			buf = buf[:runtime.Stack(buf, false)]
			logger.Criticalf("Unhandled panic: \n%v\n%s", r, buf)
			os.Exit(exit_panic)
		}
	}()

	ctx, err := cmd.DefaultContext()
	if err != nil {
		cmd.WriteError(os.Stderr, err)
		os.Exit(exit_err)
	}

	var code int
	commandName := filepath.Base(args[0])
	switch commandName {
	case jujunames.Jujud:
		code, err = jujuDMain(args, ctx)
	case jujunames.JujuRun:
		lock, err := machinelock.New(machinelock.Config{
			AgentName:   "juju-run",
			Clock:       clock.WallClock,
			Logger:      loggo.GetLogger("juju.machinelock"),
			LogFilename: filepath.Join(cmdutil.LogDir, machinelock.Filename),
		})
		if err != nil {
			code = exit_err
		} else {
			run := &run.RunCommand{MachineLock: lock}
			code = cmd.Main(run, ctx, args[1:])
		}
	case jujunames.JujuDumpLogs:
		code = cmd.Main(dumplogs.NewCommand(), ctx, args[1:])
	case jujunames.JujuIntrospect:
		code = cmd.Main(&introspect.IntrospectCommand{}, ctx, args[1:])
	default:
		code, err = hooktool.Main(commandName, ctx, args)
	}
	if err != nil {
		cmd.WriteError(ctx.Stderr, err)
	}
	return code
}

type jujudWriter struct {
	target io.Writer
}

func (w *jujudWriter) Write(entry loggo.Entry) {
	if strings.HasPrefix(entry.Module, "unit.") {
		fmt.Fprintln(w.target, w.unitFormat(entry))
	} else {
		fmt.Fprintln(w.target, loggo.DefaultFormatter(entry))
	}
}

func (w *jujudWriter) unitFormat(entry loggo.Entry) string {
	ts := entry.Timestamp.In(time.UTC).Format("2006-01-02 15:04:05")
	// Just show the last element of the module.
	lastDot := strings.LastIndex(entry.Module, ".")
	module := entry.Module[lastDot+1:]
	return fmt.Sprintf("%s %s %s %s", ts, entry.Level, module, entry.Message)
}
