# Atun - AWS Tagged Tunnel

Secure tunneling doesn't have to be hard or annoying: `atun` is a tiny cli tool which works based on remote configuration. 
It uses tags to define hosts and ports forwarding endpoints.

::: tip
While the tool works, it is still in development and versions before 1.0.0 might have breaking changes.
Be ready that commits might be squashed/reset and tags might be rewritten until 1.0.0
:::

## Features

- **Tag-Based Configuration**: Use AWS tags to define hosts and port forwarding endpoints
- **EC2 Router Support**: Connect to private resources (RDS, Redis) via EC2 instances
- **No Public IP Required**: Uses AWS Systems Manager (SSM) for secure connections
- **Simple CLI Interface**: Intuitive commands for managing tunnels
- **Cross-Platform**: Available for both macOS and Windows

[Get Started →](/guide/)
