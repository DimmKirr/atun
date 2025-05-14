# Quick Start

Get started with Atun in minutes. Follow these simple steps to set up secure tunneling to your AWS resources.

## Installation

### macOS
```bash
brew tap DimmKirr/tap
brew install atun
```

### Alpine Linux
```bash
# Add the repository key
curl https://alpine.fury.io/DimmKirr/DimmKirr@fury.io-b52e89c2.rsa.pub > /etc/apk/keys/DimmKirr@fury.io-b52e89c2.rsa.pub

# Add Atun repository
echo "https://alpine.fury.io/DimmKirr/" >> /etc/apk/repositories

# Install Atun
apk update
apk add atun
```

### Windows
```powershell
scoop bucket add DimmKirr https://github.com/DimmKirr/scoop-bucket.git
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
