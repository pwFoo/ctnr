# cntnr

CNTNR DEVELOPMENT IS IN AN EARLY STATE!

cntnr is a container engine library and CLI built on top of [runc](https://github.com/opencontainers/runc)
to manage and build OCI images as well as runtime bundles.  
cntnr aims to ease system container creation and execution as unprivileged user.
Besides cntnr is a platform to try out new runc features.


## Features
- OCI bundle and container preparation as well as execution as unprivileged user using [runc](https://github.com/opencontainers/runc)
- OCI image build as unprivileged user
- Simple concurrently accessible portable POSIX-based image and bundle store
- Image and bundle file system creation using [umoci](https://github.com/openSUSE/umoci)
- Various image formats and transports supported by [containers/image](https://github.com/containers/image)
- Optional container networking using [CNI](https://github.com/containernetworking/cni) (as OCI runtime hook)
- Partial [docker compose](https://docs.docker.com/compose/compose-file/) file format support
- Simple CLI partially compatible with [docker](https://www.docker.com/)'s
- Easy installation: single statically linked binary (plus optional CNI plugin binaries) and convention over configuration


## Rootless containers

Concerning accessibility, usability and security container engines that do not require root privileges have several advantages compared to those that do:
- **Containers can be run by unprivileged users.**  
  _Required in restrictive environments and useful for graphical applications._
- **Container images can be built almost in every Linux environment.**  
  _Higher flexibility in unprivileged CI/CD build jobs - running a container in a container is already possible (see limitations and experiments below)._
- **A higher degree and more flexible level of security.**  
  _Less likely for an attacker to gain root access through a possible engine security leak when run as unprivileged user._  
  _User/group-based container access control leveraging the host OS' ACL._

See [Aleksa Sarai's blog post](https://www.cyphar.com/blog/post/rootless-containers-with-runc) (which was an inspiration for this project) for more information.


### Limitations & challenges

Container execution as unprivileged user is limited:


**Container networks cannot be configured.**
As a result in a restrictive environment without root access only the host network can be used.
A feature on the roadmap is a daemon that runs as root and can configure a separate container namespace for an unprivileged user's container.


**Inside the container a process' or file's user cannot be changed.**
This is caused by the fact that all operations in the container are still run by the host user who is mapped to a user inside the container.
Unfortunately this stops many package managers as well as official docker images from working.  
A solution approach is to hook the kernel-space system calls and prevent their propagation to the kernel.
Though this does not solve the whole problem since applications that rely on or check the state they assume to have changed previously using such a fake system call will still not work. For this reason e.g. apt-get cannot be used in such an environment.
Fortunately dnf, yum and apk are already working with this approach in plain [runc](https://github.com/opencontainers/runc).
Other implementations are namely [fakechroot](https://github.com/dex4er/fakechroot) (using `LD_PRELOAD`), [remainroot](https://github.com/cyphar/remainroot) (using `ptrace`) and [PRoot](https://github.com/proot-me/PRoot) (using `ptrace`).
The latter can be used with cntnr when installed on your system (using `./install-proot.sh`).


## Build

Build the binary `dist/bin/cntnr` as well as `dist/bin/cni-plugins` on a Linux machine with git, make and docker:
```
git clone https://github.com/mgoltzsche/cntnr.git
cd cntnr
make
```  
Install in `/usr/local`:
```
sudo make install
```  
Optionally the project can now be opened with LiteIDE running in a cntnr container  
_(Please note that it takes some time to build the LiteIDE container image)_:
```
make ide
```


## Examples

The following examples assume your policy accepts docker images or you have copied [policy-example.json](policy-example.json) to `/etc/containers/policy.json` on your host.

### Create and run container from Docker image
```
> cntnr run docker://alpine:3.7 echo hello world
hello world
```

### Create and run Firefox as unprivileged user
Build a Firefox ESR container image `local/firefox:alpine` (cached operation):
```
cntnr image create \
	--from=docker://alpine:3.7 \
	--author='John Doe' \
	--run='apk add --update --no-cache firefox-esr libcanberra-gtk3 adwaita-icon-theme ttf-ubuntu-font-family' \
	--cmd=firefox \
	--tag=local/firefox:alpine
```  

Create and run a bundle named `firefox` from the previously built image:
```
cntnr run -b firefox --update \
	--env DISPLAY=$DISPLAY \
	--mount /tmp/.X11-unix:/tmp/.X11-unix \
	--mount /etc/machine-id:/etc/machine-id:ro \
	local/firefox:alpine
```  
The `-b <BUNDLE>` and `--update` options make this operation idempotent:
The bundle's file system is reused and only recreated when the underlying image has changed.
Use these options to restart containers very quickly. Without them cntnr copies the
image file system on bundle creation which can take some time and disk space depending on the image's size.  
Also these options enable a container update on restart when the base image is frequently updated before the child image is rebuilt using the following command:
```
cntnr image import docker://alpine:3.7
```


## The OCI standard and this implementation

An *[OCI image](https://github.com/opencontainers/image-spec/tree/v1.0.0)* basically provides a base [configuration](https://github.com/opencontainers/image-spec/blob/v1.0.0/config.md) and file system to create an OCI bundle from. The file system consists of a list of layers which are represented by archive files each containing the diff to its parent.  
cntnr manages images in its local store directory in the [OCI image layout format](https://github.com/opencontainers/image-spec/blob/v1.0.0/image-layout.md).
Images are imported into the local store using the [containers/image](https://github.com/containers/image) library.
A new bundle is created by extracting the image's file system into a directory using [umoci](https://github.com/openSUSE/umoci)
and [deriving](https://github.com/opencontainers/image-spec/blob/v1.0.0/conversion.md) the bundle's default configuration from the image's configuration.


An *[OCI bundle](https://github.com/opencontainers/runtime-spec/blob/v1.0.0/bundle.md)*
provides the [configuration](https://github.com/opencontainers/runtime-spec/blob/v1.0.0/config.md) and the file system required to create a container.
Basically it is a directory containing a `config.json` file with the configuration and a sub directory with the file system.  
cntnr manages bundles in its local store directory. Alternatively a custom directory can also be used as bundle.


An *[OCI container](https://github.com/opencontainers/runtime-spec/blob/v1.0.0/runtime.md)* is a host-specific bundle instance.
On Linux it is a set of namespaces in which a configured process can be run.  
cntnr uses [runc/libcontainer](https://github.com/opencontainers/runc/blob/v1.0.0-rc4/libcontainer/README.md) as OCI runtime implementation.


## Related tools

- [docker](https://www.docker.com/)
- [rkt](https://rkt.io)
- [runc](https://github.com/opencontainers/runc), [skopeo](https://github.com/projectatomic/skopeo), [umoci](https://github.com/openSUSE/umoci)
- [udocker](https://github.com/indigo-dc/udocker)
- [runrootless](https://github.com/AkihiroSuda/runrootless)
- [singularity](http://singularity.lbl.gov/)

## Roadmap

- separate OCI hook binary
- CLI improvements: image rm, image build, bundle run, compose
- additional configurable read-only image stores
- Improved container, bundle and image garbage collection
- health check
- systemd integration (cgroup, startup notification)
- network manager daemon with ACL to be used by unprivileged users to configure their container networks
- service discovery integration (hook / DNS; consul, etcd)
- _Far future: Make it available on platforms other than Linux_

## Experiments

[Experiments with nested containers](experiments.md)
