---
# https://brenoepics.github.io/vitepress-carbon/guide/home-component.html
layout: home

hero:
  name: "Atun"
  text: "Tunnels on Private Bastions"
  tagline: Seamless, IAM-native access to private RDS, Elasticache, DynamoDB, and more. No VPNs, no SSH agents, no friction
  icon: 🐟️
  image:
    src: ./demo/up.cast.svg
    alt: Banner
    width: 1000
    height: 435
  actions:
    - theme: brand
      text: Quickstart
      link: /guide/quickstart
    - theme: alt
      text: View on Github
      link: https://github.com/automationd/atun


features:
  - title: Tag-Based Configuration 
    details: Use AWS tags to define hosts and port forwarding endpoints
  - title: EC2 Router Support
    details: Connect to private resources (RDS, Redis) via EC2 instances
  - title: No Public IP Required
    details: Uses AWS Systems Manager (SSM) for secure connections
---
