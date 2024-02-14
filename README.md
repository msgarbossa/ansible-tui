# ansible-shim

## Description

ansible-shim is a lightweight CLI wrapper written in Go for running ansible-playbook.  ansible-shim provides both an environment-variable and YAML based API to enable ansible-playbook to be run by other tools and services.

## Usage

See [setup](#setup) instructions below for dependencies and first-time setup.

ansible-shim first looks for a YAML configuration file with the playbook execution parameters.  The path to the YAML configuration file can be specified with either PB_CONFIG_FILE environment variable or with the -c command line option.

### Parameters

- The final configuration must minimally include the playbook and inventory file.
- Environment variables have higher precedence and can override parameters in the YAML configuration file.
- ENV Parameters are all environment variables and therefore strings.
- YAML parameters are all strings unless otherwise noted (verbose_level).

| ENV Parameter | YAML Parameter | Purpose | ansible-playbook CLI option |
| ------------- | -------------- | ------- | --------------------------- |
| PB_CONFIG_FILE | NA | Relative path to YAML configuration which is read before environment variables are processed (can also be passed to ansible-shim with -c) | NA |
| PLAYBOOK | playbook | Relative path to the playbook to execute | NA |
| VERBOSE_LEVEL | verbose_level (int) | String containing 0-4, corresponding the number of v's controlling the level of verbosity | -v, -vv, -vvv, -vvvv |
| SSH_PRIVATE_KEY_FILE | ssh_private_key_file | Path to SSH private key (for SSH connections only) | NA |
| ANSIBLE_REMOTE_USER | remote_user | Remote user for target machine | NA |
| INVENTORY_FILE | inventory | Absolute or relative path to inventory file (no backward traversal w/ "..") | -i |
| INVENTORY_CONTENTS | NA | Multi-line string containing inventory contents.  Contents are written to a file and passed via -i ./hosts-INVENTORY | NA |
| INVENTORY_URL | NA | Retrieves a single Ansible inventory file from a URL to be used as INVENTORY_FILE | NA |
| LIMIT_HOST | limit | Limit targets hosts to a host or group name or pattern resolved in Ansible inventory | --limit |
| EXTRA_VARS_FILE | extra_vars_file | Absolute or relative path to extra-vars file (no backward traversal w/ "..") | -e --extra-vars |
| EXTRA_VARS_CONTENTS | NA | Multi-line string containing extra-vars contents.  Contents are written to a file and passed via -e ./PLAYBOOK-extravars | NA |
| ANSIBLE_TAGS | tags | Run Ansible tasks with specific tag values (TBD) | --tags |
| ANSIBLE_SKIP_TAGS | skip_tags | Skip Ansible tasks with specific tag values (TBD) | --skip-tags |
| EXTRA_ARGS | extra_args | Additional options appended to ansible-playbook command (TBD) | NA |
| WINDOWS_GROUP | windows_group | Group name in Ansible inventory where WinRM should be used with WinRM parameters (TBD) | NA |
| VIRTUAL_ENV | virtual_env_path | Path to Python virtual environment directory (must contain ./bin/ansible-playbook) | NA |
| CONTAINER_IMAGE | image | Container image URI with ansible-shim, ansible-playbook, and any other playbook runtime dependencies | NA |
| ANSIBLE_PLAYBOOK_TIMEOUT | NA | Number of seconds to timeout playbook execution | NA |

- Only one INVENTORY_ parameter is required
- Only one EXTA_VARS_ parameter can be specified
- Either VIRTUAL_ENV or CONTAINER_IMAGE can be specified.  In a container image, ansible-playbook must be in the environment's PATH.

### YAML Configuration file

All configuration file arguments are initialized to empty-string or 0 based on the variable type.  Only non-empty configurations need to be specified in the configuration file.

```yaml
---
virtual_env_path: "~/Documents/venv/ansible-latest"
ssh_private_key_file: "~/.ssh/id_rsa"
inventory: "./examples/hosts.yml"
playbook: "./examples/site.yml"
# image: ansible-shim:latest
```

## Setup

### Dependencies

To build project:
- [Taskfile](#taskfile-setup)
- [go](#go-setup) >= 1.22.0

The runtime for ansible can use a globally installed ansible, a virtual environment, or a container.

For playbook execution (local):
- ansible-playbook (prefer Python virtual environment instead of global, see [python setup](#python-virtual-environment-setup) below)

For playbook execution (container):
- Docker or Podman (podman will be used if both are found, see [container setup](#container-runtime) below)


### Taskfile setup

https://taskfile.dev/installation/

Download and put the file in the PATH (ex. /usr/local/bin/task).

Ensure it is executable (chmod +x /usr/local/bin/task)


### Go setup

https://go.dev/doc/install

Go 1.22.0 or higher is required due to the use of features introduced in 1.22.0.

The basic setup for go is to download the tar file to /usr/local and unpack to /usr/local/go.

Then setup your profile (~/.profile, ~/.bash_profile, ~/.bashrc) to include the following:

```bash
# set variables for go if it exists
if [ -d "/usr/local/go/bin" ] ; then
    PATH="$PATH:/usr/local/go/bin:$HOME/go/bin"
    export GOPATH=~/go
    export GOROOT=/usr/local/go
fi
```

### Python virtual environment setup

To create a virtual environment with python modules for Ansible (Debian/Ubuntu):

```bash
apt-get install -y python3-virtualenv
mkdir ~/Documents/venv
virtualenv -p python3 ~/Documents/venv/ansible-latest
source ~/Documents/venv/ansible-latest/bin/activate
pip3 install -r ./test/requirements.txt
```

### Container runtime

The included Dockerfile builds a basic Ansible runtime environment and includes ansible-shim.

Docker or Podman can be used to execute ansible inside a container.  If both Docker and Podman are installed, Podman is used.

Docker and Podman use separate image caches.  Docker will not find a local container in the Podman registry and vice-versa.  The image should be pushed to an external registry.

When building the image, the default container name and tag are specified in the `vars` section at the top of Taskfile.yml.

#### Docker

```bash
task docker_build
```

#### Podman


```bash
task podman_build
```
