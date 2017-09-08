package run

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/mgoltzsche/cntnr/log"
)

type ContainerInfo struct {
	ID     string
	Status string
}

type ContainerManager struct {
	runners map[string]Container
	rootDir string
	debug   log.Logger
}

func NewContainerManager(rootDir string, debug log.Logger) (*ContainerManager, error) {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, err
	}
	return &ContainerManager{map[string]Container{}, absRoot, debug}, nil
}

func (m *ContainerManager) NewContainer(id, bundleDir string, terminal, bindStdin bool) (Container, error) {
	/*c := exec.Command("runc", "--root", rootDir, "create", id)
	c.Dir = runtimeBundleDir
	c.Stderr = os.Stderr
	c.Stdout = os.Stdout
	err := c.Run()
	if err != nil {
		return nil, fmt.Errorf("Error: runc container creation: %s", err)
	}*/

	if id == "" {
		id = GenerateId()
	}
	c := exec.Command("runc", "--root", m.rootDir, "run", id)
	c.Dir = bundleDir
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	if bindStdin || terminal {
		c.Stdin = os.Stdin
	}

	if !terminal {
		// Run in separate process group to be able to control orderly shutdown
		c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}

	return &RuncContainer{id, c, m.debug}, nil
}

func (m *ContainerManager) Kill(id, signal string, all bool) error {
	var args []string
	if all {
		args = []string{"--root", m.rootDir, "kill", "--all=true", id, signal}
	} else {
		args = []string{"--root", m.rootDir, "kill", id, signal}
	}
	c := exec.Command("runc", args...)
	var buf bytes.Buffer
	c.Stdout = &buf
	c.Stderr = &buf
	if err := c.Run(); err != nil {
		return fmt.Errorf("%s: %s", err, strings.TrimRight(buf.String(), "\n"))
	}
	return nil
}

func (m *ContainerManager) Deploy(c Container) error {
	err := c.Start()
	if err == nil {
		m.runners[c.ID()] = c
	}
	return err
}

func (m *ContainerManager) Stop() (err error) {
	for _, c := range m.runners {
		e := c.Stop()
		if e != nil {
			m.debug.Printf("Failed to stop container %s: %v", c.ID(), err)
			if err == nil {
				err = e
			}
		}
	}
	return err
}

func (m *ContainerManager) Wait() (err error) {
	for _, c := range m.runners {
		e := c.Wait()
		if e != nil {
			m.debug.Printf("Failed to wait for container %s: %v", c.ID(), err)
			if err == nil {
				err = e
			}
		}
	}
	return err
}

func (m *ContainerManager) List() (r []ContainerInfo, err error) {
	r = []ContainerInfo{}
	if _, e := os.Stat(m.rootDir); !os.IsNotExist(e) {
		files, err := ioutil.ReadDir(m.rootDir)
		if err == nil {
			for _, f := range files {
				if _, e := os.Stat(filepath.Join(m.rootDir, f.Name(), "state.json")); !os.IsNotExist(e) {
					r = append(r, ContainerInfo{f.Name(), "running"})
				}
			}
		}
	}
	return
}

func (m *ContainerManager) HandleSignals() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)
	go func() {
		<-sigs
		err := m.Stop()
		if err != nil {
			os.Stderr.WriteString(fmt.Sprintf("Failed to stop: %s\n", err))
		}
	}()
}
