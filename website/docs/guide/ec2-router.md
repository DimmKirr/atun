# EC2 Router Configuration

An EC2 router is a crucial component that enables Atun to establish secure connections to private AWS resources.

## What is an EC2 Router?

An EC2 router is a small EC2 instance that acts as a proxy for your tunneling connections. It allows you to:
- Access private RDS instances
- Connect to ElastiCache clusters
- Reach any other resources in private subnets

## Setting Up a Router

There are multiple ways to set up and use a router in Atun:

### 1. **Create a New Router**
```bash
atun router create
```
This command creates a new EC2 instance configured as an Atun router with all necessary tags and configurations.
It's recommended to use this for ad-hoc connections when there is no existing router and 

### 2. **Install on Existing Instance**
```bash
atun router install --router <instance-id>
```
Use this command to convert an existing EC2 instance into an Atun router by installing required components and configurations.

### 3. **Terraform Configuration**
It's possible to configure the router using Terraform tags. Here's an example of how to configure an EC2 instance as a router using Terraform
> [!Note]
> This example assumes you have an RDS instance managed via module.rds

```hcl
resource "aws_instance" "router" {
  ami           = "ami-0c55b159cbfafe1f0"
  instance_type = "t3.micro"
  subnet_id     = aws_subnet.private.id
  key_name      = aws_key_pair.my_key.key_name
  tags = {
   "atun.io/version" = "1"
   "atun.io/env" = "prod"     
   "atun.io/host/${module.rds.cluster_endpoint}" = jsonencode({
      local    = 10001
      remote   = module.rds.cluster_port
      protocol = "ssm"
   })
  }
}
```

### 4. **Manual Configuration**
It's also possible to manually configure any EC2 instance as a router by adding the required [Atun tags](./tag-schema.md) to the instance.
Not a very scalable option, but it gives you full control over the instance configuration while still integrating with Atun's routing system.

