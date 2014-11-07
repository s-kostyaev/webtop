# $Id: pkgbuild-mode.el,v 1.23 2007/10/20 16:02:14 juergen Exp $
# Maintainer:  <s-kostyaev@localhost>
pkgname=webtop-git
pkgver=0.1
pkgrel=1
pkgdesc="web-based top for cgroup"
arch=('i686' 'x86_64')
url="https://github.com/s-kostyaev/webtop"
license=('unknown')
depends=()
makedepends=('go')
backup=('etc/webtop.toml')
branch='dev'
source=("${pkgname}::git+https://github.com/s-kostyaev/webtop#branch=${branch}")
md5sums=('SKIP')
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
}

