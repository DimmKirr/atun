# Quick Start

Get started with Atun in minutes. Follow these simple steps to set up secure tunneling to your AWS resources.

## Installation

### macOS
```bash
brew tap automationd/tap
brew install atun
```

### Windows
```powershell
scoop bucket add automationd https://github.com/automationd/scoop-bucket.git
scoop install atun
```

## Basic Usage
### Start a Tunnel
```bash
atun up
```

### Check Status
```bash
atun status
```

### Stop the Tunnel
```bash
atun down
```

## Next Steps

- Learn about [EC2 Router](/guide/ec2-router)
- Understand the [Tag Schema](/guide/tag-schema)
- Explore [CLI Commands](/reference/cli-commands)
