# Tag Schema Reference

Atun uses AWS resource tags with the `atun.io` schema to configure tunneling endpoints. This page describes the tag schema and supported values.

## Core Tags

| Tag | Description | Example | Required |
|-----|-------------|---------|----------|
| `atun.io/version` | Schema version | `1` | Yes |
| `atun.io/env` | Environment name | `dev` | Yes |
| `atun.io/host/<hostname>` | Host endpoint configuration | See below | Yes |

## Host Tag Format

The host tag value is a JSON object with the following structure:
```json
{
    "local": "<local_port>",
    "proto": "<protocol>",
    "remote": <remote_port>
}
```

### Fields
- `local`: Port that will be bound on your local machine
- `proto`: Protocol for forwarding (currently only `ssm` is supported)
- `remote`: Port that is available on the internal network to the router host

## Examples

### RDS Instance
```
Tag Key: atun.io/host/nutcorp-api.cluster-xxxxxxxxxxxxxxx.us-east-1.rds.amazonaws.com
Tag Value: {"local":"23306","proto":"ssm","remote":3306}
```

### Redis Cluster
```
Tag Key: atun.io/host/nutcorp.xxxxxx.0001.use0.cache.amazonaws.com
Tag Value: {"local":"26379","proto":"ssm","remote":6379}
```
