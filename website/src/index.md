---
# https://brenoepics.github.io/vitepress-carbon/guide/home-component.html
layout: home

hero:
#  name: "Atun"
  text: "Private Bastion Tunnels Simplified"
  tagline: Seamless, secure access to a private RDS, Elasticache, DynamoDB, or any other resource. No VPNs, no SSH agents, no friction
#  logo: ./demo/up.hero.svg
  icon: 🐟️
  image:
#    src: ./logo.png
    src: ./demo/up.hero.svg
    alt: Banner
##    width: 750
#    height: 435
    
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
    icon: 🏷️
  - title: EC2 Router Support
    details: Connect to private resources (RDS, Redis) via EC2 instances
    icon: ⚡
  - title: No Public IP Required
    details: Uses AWS Systems Manager (SSM) for secure connections
    icon: 🔒
---
