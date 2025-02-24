package connect

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"google.golang.org/grpc"
	empty "google.golang.org/protobuf/types/known/emptypb"

	rpc "github.com/telepresenceio/telepresence/rpc/v2/daemon"
	"github.com/telepresenceio/telepresence/v2/pkg/client"
	"github.com/telepresenceio/telepresence/v2/pkg/client/cli/daemon"
	"github.com/telepresenceio/telepresence/v2/pkg/client/cli/output"
	"github.com/telepresenceio/telepresence/v2/pkg/client/socket"
	"github.com/telepresenceio/telepresence/v2/pkg/errcat"
	"github.com/telepresenceio/telepresence/v2/pkg/filelocation"
	"github.com/telepresenceio/telepresence/v2/pkg/proc"
)

func launchDaemon(ctx context.Context, cr *daemon.Request) error {
	fmt.Fprintln(output.Info(ctx), "Launching Telepresence Root Daemon")

	// Ensure that the logfile is present before the daemon starts so that it isn't created with
	// root permissions.
	logDir := filelocation.AppUserLogDir(ctx)
	logFile := filepath.Join(logDir, "daemon.log")
	if _, err := os.Stat(logFile); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err = os.MkdirAll(logDir, 0o700); err != nil {
			return err
		}
		fh, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		_ = fh.Close()
	}

	args := []string{client.GetExe(), "daemon-foreground"}
	if cr != nil && cr.RootDaemonProfilingPort > 0 {
		args = append(args, "--pprof", strconv.Itoa(int(cr.RootDaemonProfilingPort)))
	}
	args = append(args, logDir, filelocation.AppUserConfigDir(ctx))
	return proc.StartInBackgroundAsRoot(ctx, args...)
}

// ensureRootDaemonRunning ensures that the daemon is running.
func ensureRootDaemonRunning(ctx context.Context) error {
	if ud := daemon.GetUserClient(ctx); ud != nil && ud.Remote() {
		// Never start root daemon when running remote
		return nil
	}
	cr := daemon.GetRequest(ctx)
	if cr != nil && cr.Docker {
		// Never start root daemon when connecting using a docker container.
		return nil
	}
	if addr := client.GetEnv(ctx).UserDaemonAddress; addr != "" {
		// Always assume that root daemon is running when a user daemon address is provided
		return nil
	}
	running, err := socket.IsRunning(ctx, socket.RootDaemonPath(ctx))
	if err != nil || running {
		return err
	}
	if err = launchDaemon(ctx, cr); err != nil {
		return fmt.Errorf("failed to launch the daemon service: %w", err)
	}
	if err = socket.WaitUntilRunning(ctx, "daemon", socket.RootDaemonPath(ctx), 10*time.Second); err != nil {
		return fmt.Errorf("daemon service did not start: %w", err)
	}
	return nil
}

// Disconnect shuts down a session in the root daemon. When it shuts down, it will tell the connector to shut down.
func Disconnect(ctx context.Context, quitDaemons bool) error {
	err := UserDaemonDisconnect(ctx, quitDaemons)
	if errors.Is(err, ErrNoUserDaemon) {
		err = nil
	}
	if err != nil {
		return fmt.Errorf("error when quitting connector: %w", err)
	}
	if quitDaemons {
		// User daemon is responsible for killing the root daemon, but we kill it here too to cater for
		// the fact that the user daemon might have been killed ungracefully.
		if err = socket.WaitUntilVanishes("root daemon", socket.RootDaemonPath(ctx), 5*time.Second); err != nil {
			var conn *grpc.ClientConn
			if conn, err = socket.Dial(ctx, socket.RootDaemonPath(ctx)); err == nil {
				if _, err = rpc.NewDaemonClient(conn).Quit(ctx, &empty.Empty{}); err != nil {
					err = fmt.Errorf("error when quitting root daemon: %w", err)
				}
			}
		}
	}
	return err
}

func mkdir(dirType, path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return errcat.NoDaemonLogs.Newf("unable to ensure that %s directory %q exists: %w", dirType, path, err)
	}
	return nil
}

func ensureAppUserCacheDirs(ctx context.Context) error {
	cacheDir := filelocation.AppUserCacheDir(ctx)
	if err := mkdir("cache", filepath.Join(cacheDir, "daemons")); err != nil {
		return err
	}
	if err := mkdir("cache", filepath.Join(cacheDir, "kube")); err != nil {
		return err
	}
	if err := mkdir("cache", filepath.Join(cacheDir, "sessions")); err != nil {
		return err
	}
	return nil
}

func ensureAppUserConfigDir(ctx context.Context) error {
	configDir := filelocation.AppUserConfigDir(ctx)
	if err := mkdir("config", filepath.Join(configDir, "sessions")); err != nil {
		return err
	}
	return nil
}
