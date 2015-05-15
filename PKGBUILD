# Maintainer:  <s-kostyaev@ngs>
pkgname=webtop-git
pkgver=1.0.0
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
build(){
  cd ${srcdir}/${pkgname}
  deps=`go list -f '{{join .Deps "\n"}}' |  xargs go list -f '{{if not .Standard}}{{.ImportPath}}{{end}}'`
  for dep in $deps; do go get $dep; done
  go build -o webtop
}
package(){
  install -D -m 755 ${srcdir}/${pkgname}/webtop ${pkgdir}/usr/bin/webtop
  install -D -m 644 ${srcdir}/${pkgname}/webtop.toml ${pkgdir}/etc/webtop.toml
  install -D -m 644 ${srcdir}/${pkgname}/webtop.service ${pkgdir}/usr/lib/systemd/system/webtop.service
}

