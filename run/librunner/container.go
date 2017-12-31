package librunner

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/coreos/go-systemd/activation"
	"github.com/mgoltzsche/cntnr/log"
	"github.com/mgoltzsche/cntnr/run"
	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/runc/libcontainer/configs"
	_ "github.com/opencontainers/runc/libcontainer/nsenter"
	"github.com/opencontainers/runc/libcontainer/specconv"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

func init() {
	if len(os.Args) > 1 && os.Args[1] == "init" {
		runtime.GOMAXPROCS(1)
		runtime.LockOSThread()
		factory, _ := libcontainer.New("")
		if err := factory.StartInitialization(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: libcontainer factory initialization: %s\n", err)
			os.Exit(1)
		}
		panic("factory initialization should block further execution - this should never be executed")
	}
}

type Container struct {
	container libcontainer.Container
	process   *libcontainer.Process
	tty       *tty
	io        run.ContainerIO
	id        string
	bundle    run.ContainerBundle
	spec      *specs.Spec
	mutex     *sync.Mutex
	wait      *sync.WaitGroup
	debug     log.Logger
	err       error
}

func NewContainer(id string, bundle run.ContainerBundle, ioe run.ContainerIO, rootless bool, factory libcontainer.Factory, debug log.Logger) (r *Container, err error) {
	if id == "" {
		if id = bundle.ID(); id == "" {
			panic("no container ID provided and bundle ID is empty")
		}
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	defer func() {
		if err != nil {
			err = errors.Wrap(err, "new container")
		}
	}()

	spec, err := bundle.Spec()
	if err != nil {
		return
	}
	if spec.Process == nil {
		return nil, fmt.Errorf("bundle spec declares no process to run")
	}
	orgwd, err := os.Getwd()
	if err != nil {
		return
	}
	if err = os.Chdir(bundle.Dir()); err != nil {
		return nil, fmt.Errorf("change to bundle directory: %s", err)
	}
	defer func() {
		if e := os.Chdir(orgwd); e != nil {
			err = fmt.Errorf("change back from bundle to previous directory: %s", e)
		}
	}()

	config, err := specconv.CreateLibcontainerConfig(&specconv.CreateOpts{
		CgroupName:       id,
		UseSystemdCgroup: false,
		NoPivotRoot:      false,
		NoNewKeyring:     false,
		Spec:             spec,
		Rootless:         rootless,
	})
	if err != nil {
		return
	}
	if spec.Root != nil {
		config.Rootfs = filepath.Join(bundle.Dir(), spec.Root.Path)
	}
	container, err := factory.Create(id, config)
	if err != nil {
		return
	}

	r = &Container{
		container: container,
		id:        id,
		io:        ioe,
		bundle:    bundle,
		spec:      spec,
		mutex:     &sync.Mutex{},
		wait:      &sync.WaitGroup{},
		debug:     debug,
	}

	return
}

func (c *Container) ID() string {
	return c.id
}

// Prepare and start the container process from spec and with stdio
func (c *Container) Start() (err error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	defer func() {
		if err != nil {
			err = fmt.Errorf("start %q: %s", c.container.ID(), err)
		}
	}()

	if c.process != nil {
		return fmt.Errorf("container already started")
	}

	// Create container process (see https://github.com/opencontainers/runc/blob/v1.0.0-rc4/utils_linux.go: startContainer->runner.run->newProcess)
	p := c.spec.Process
	lp := &libcontainer.Process{
		Args:   p.Args,
		Env:    p.Env,
		User:   fmt.Sprintf("%d:%d", p.User.UID, p.User.GID),
		Cwd:    p.Cwd,
		Stdout: c.io.Stdout,
		Stderr: c.io.Stderr,
		Stdin:  c.io.Stdin,
	}
	for _, gid := range p.User.AdditionalGids {
		lp.AdditionalGroups = append(lp.AdditionalGroups, strconv.FormatUint(uint64(gid), 10))
	}
	if p.Capabilities != nil {
		lp.Capabilities = &configs.Capabilities{}
		lp.Capabilities.Bounding = p.Capabilities.Bounding
		lp.Capabilities.Effective = p.Capabilities.Effective
		lp.Capabilities.Inheritable = p.Capabilities.Inheritable
		lp.Capabilities.Permitted = p.Capabilities.Permitted
		lp.Capabilities.Ambient = p.Capabilities.Ambient
	}
	if p.Rlimits != nil {
		for _, rlimit := range p.Rlimits {
			rl, err := createLibContainerRlimit(rlimit)
			if err != nil {
				return err
			}
			lp.Rlimits = append(lp.Rlimits, rl)
		}
	}
	if os.Getenv("LISTEN_FDS") != "" {
		// Add systemd file descriptors
		lp.ExtraFiles = activation.Files(false)
	}

	// Configure stdIO/terminal
	tty, err := setupIO(lp, c.container, p.Terminal, false, "")
	if err != nil {
		return
	}

	// Run container process
	if err = c.container.Run(lp); err != nil {
		return fmt.Errorf("spawn main process: %s", err)
	}
	c.process = lp
	c.tty = tty

	c.wait.Add(1)
	go c.handleProcessTermination()

	return
}

func (c *Container) handleProcessTermination() {
	defer c.wait.Done()

	// Wait for process
	_, err := c.process.Wait()
	c.err = run.NewExitError(err)

	// Release TTY
	err = c.tty.Close()
	c.err = run.WrapExitError(c.err, err)
	c.tty = nil

	c.debug.Printf("Container %q terminated", c.ID())
}

func (c *Container) Stop() {
	if c.process == nil {
		return
	}

	go c.stop()
}

func (c *Container) stop() bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.process == nil {
		return false
	}

	// Terminate container orderly
	c.debug.Printf("Terminating container %q...", c.ID())
	if err := c.process.Signal(syscall.SIGINT); err != nil {
		c.debug.Printf("Failed to send SIGINT to container %q process: %s", c.ID(), err)
	}

	quit := make(chan error, 1)
	go func() {
		quit <- c.Wait()
	}()
	select {
	case <-time.After(time.Duration(10000000)): // TODO: read value from OCI runtime configuration
		// Kill container after timeout
		if c.process != nil {
			c.debug.Printf("Killing container %q process since stop timeout exceeded", c.ID())
			if err := c.process.Signal(syscall.SIGKILL); err != nil {
				err = fmt.Errorf("stop: killing container %q: %s", c.ID(), err)
			}
			c.Wait()
		}
		<-quit
	case <-quit:
	}
	close(quit)
	c.process = nil
	return true
}

func (c *Container) Wait() error {
	c.wait.Wait()
	return c.err
}

func (c *Container) Close() (err error) {
	if c.stop() {
		err = c.err
	}
	if e := c.container.Destroy(); e != nil {
		err = run.WrapExitError(err, e)
	}
	if e := c.bundle.Close(); e != nil {
		err = run.WrapExitError(err, e)
	}
	c.container = nil
	return err
}
