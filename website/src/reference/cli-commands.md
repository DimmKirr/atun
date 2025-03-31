# CLI Commands Reference

This page lists all available Atun CLI commands and their usage. Atun is an SSH tunnel CLI tool that works without local configuration, using EC2 tags to define hosts and ports forwarding configuration.

## Global Flags

These flags are available for all commands:

- `--aws-profile string`: Specify AWS profile (defined in ~/.aws/credentials)
- `--aws-region string`: Specify AWS region (e.g. us-east-1)
- `--env string`: Specify environment (dev/prod/...)
- `--log-level string`: Specify log level (debug/info/warn/error)

## Core Commands

### `atun up`
Starts a tunnel to the router host and forwards ports to the local machine.

```bash
atun up [flags]
```

**Flags:**
- `-c, --create`: Create ad-hoc router if it doesn't exist (managed by built-in CDKTf)
- `-r, --router string`: Router instance ID to use (defaults to first running instance with atun.io tags)

### `atun down`
Bring the existing tunnel down.

```bash
atun down [flags]
```

**Flags:**
- `-x, --delete`:  Delete ad-hoc router (if exists). Won't delete any resources non-managed by atun
- `-r, --router string`: Router instance id to use. If not specified the first running instance with the atun.io tags is used

### `atun status`
Show status of the tunnel and current environment.

```bash
atun status [flags]
```

**Flags:**
- `-d, --detailed`:  Show detailed status

### `atun version`
Display version information.

## Router Management
### `atun router create`
Creates an ad-hoc router host in a specified subnet.

### `atun router install`
Install Atun tags on an existing EC2 instance.

### `atun router uninstall`
Remove Atun tags from an EC2 instance.

### `atun router delete`
Deletes an ad-hoc router host.

### `atun router ls`
List available routers.

### `atun router shell`
Connect directly to a router endpoint via SSH.

## Additional Commands

### `atun completion [command]`
Generate the autocompletion script for the specified shell.

**Flags:**
- `bash` - Generate the autocompletion script for bash
- `zsh` - Generate the autocompletion script for zsh
- `fish` - Generate the autocompletion script for fish
- `powershell` - Generate the autocompletion script for powershell

