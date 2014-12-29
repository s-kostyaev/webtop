# Maintainer:  <s-kostyaev@ngs>
pkgname=webtop-git
pkgver=0.3.1
pkgrel=1
pkgdesc="web-based top for cgroup"
arch=('i686' 'x86_64')
url="https://github.com/s-kostyaev/webtop"
license=('unknown')
depends=('git')
makedepends=('go')
backup=('etc/webtop.toml')
branch='dev'
source=("${pkgname}::git+https://github.com/s-kostyaev/webtop#branch=${branch}")
md5sums=('SKIP')
install=webtop.install
build(){
      go get github.com/BurntSushi/toml
	  go get github.com/op/go-logging
	  go get github.com/brnv/go-heaver
	  go get github.com/s-kostyaev/go-iptables-proxy
	  go get github.com/s-kostyaev/go-lxc
	  go get github.com/shirou/gopsutil
	  cd ${srcdir}/${pkgname}
	  go build -o webtop
}
package(){
  install -D -m 755 ${srcdir}/${pkgname}/webtop ${pkgdir}/usr/bin/webtop
  install -D -m 644 ${srcdir}/${pkgname}/webtop.toml ${pkgdir}/etc/webtop.toml
  install -D -m 644 ${srcdir}/${pkgname}/top.htm ${pkgdir}/usr/share/webtop/top.htm
  install -D -m 644 ${srcdir}/${pkgname}/webtop.service ${pkgdir}/usr/lib/systemd/system/webtop.service
  install -D -m 644 ${srcdir}/${pkgname}/12-ip-forward.conf ${pkgdir}/etc/sysctl.d/12-ip-forward.conf
}

