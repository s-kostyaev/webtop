webtop
=======

This simple daemon monitor memory usage of lxc containers throw cgrop on current
host. If memory usage reach the limit daemon enable port forwarding
with iptables and redirect from container ip 80 port to webtop. In web
interface user of this container (tester or developer) can see
container's processes with process memory usage and key for kill this
process. Even memory get available daemon disable port forwarding.
