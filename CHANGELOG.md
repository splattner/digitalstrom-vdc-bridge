# Changelog

## [0.1.8](https://github.com/splattner/digitalstrom-vdc-bridge/compare/v0.1.7...v0.1.8) (2026-05-17)


### Bug Fixes

* use hostnetwork, otherwise mdns does not work ([#40](https://github.com/splattner/digitalstrom-vdc-bridge/issues/40)) ([67803f7](https://github.com/splattner/digitalstrom-vdc-bridge/commit/67803f7e3524c672148bd8c3620a5fa8d8eda050))

## [0.1.7](https://github.com/splattner/digitalstrom-vdc-bridge/compare/v0.1.6...v0.1.7) (2026-05-17)


### Features

* exteral device api as plugin ([#37](https://github.com/splattner/digitalstrom-vdc-bridge/issues/37)) ([040c114](https://github.com/splattner/digitalstrom-vdc-bridge/commit/040c114408c2c38432d1f035132674164f80dd16))
* select mqtt broker in frontend, forward vendor / model, show ha intgration ([#39](https://github.com/splattner/digitalstrom-vdc-bridge/issues/39)) ([8d22b5d](https://github.com/splattner/digitalstrom-vdc-bridge/commit/8d22b5d4cc2902c274e1719303a0f430dc091321))


### Bug Fixes

* fallback to direct mdsn if avahi not available ([#35](https://github.com/splattner/digitalstrom-vdc-bridge/issues/35)) ([2ab00a7](https://github.com/splattner/digitalstrom-vdc-bridge/commit/2ab00a72920a0b53a9281560042b0a301c78b97a))

## [0.1.6](https://github.com/splattner/digitalstrom-vdc-bridge/compare/v0.1.5...v0.1.6) (2026-05-16)


### Bug Fixes

* use dbus/avahi when running as ha addon ([#33](https://github.com/splattner/digitalstrom-vdc-bridge/issues/33)) ([8fb5495](https://github.com/splattner/digitalstrom-vdc-bridge/commit/8fb5495d3a3d871dfa9e4ad230597ae5a2b27e92))

## [0.1.5](https://github.com/splattner/digitalstrom-vdc-bridge/compare/v0.1.4...v0.1.5) (2026-05-16)


### Bug Fixes

* make it work inside ha as addon ([#31](https://github.com/splattner/digitalstrom-vdc-bridge/issues/31)) ([5897e72](https://github.com/splattner/digitalstrom-vdc-bridge/commit/5897e726c7130ce0b6942be689861496e020a666))

## [0.1.4](https://github.com/splattner/digitalstrom-vdc-bridge/compare/v0.1.3...v0.1.4) (2026-05-16)


### Bug Fixes

* prepare is needed ([#29](https://github.com/splattner/digitalstrom-vdc-bridge/issues/29)) ([62a2fb8](https://github.com/splattner/digitalstrom-vdc-bridge/commit/62a2fb88cd43f1749f01bb538fa4078fa3876d2b))

## [0.1.3](https://github.com/splattner/digitalstrom-vdc-bridge/compare/v0.1.2...v0.1.3) (2026-05-16)


### Bug Fixes

* wrong bae image ([#27](https://github.com/splattner/digitalstrom-vdc-bridge/issues/27)) ([7eed43a](https://github.com/splattner/digitalstrom-vdc-bridge/commit/7eed43a3ecb0e009a3a7f2046f56d164621aedc4))

## [0.1.2](https://github.com/splattner/digitalstrom-vdc-bridge/compare/v0.1.1...v0.1.2) (2026-05-16)


### Bug Fixes

* ha addon should use correct image ([#25](https://github.com/splattner/digitalstrom-vdc-bridge/issues/25)) ([08cbdca](https://github.com/splattner/digitalstrom-vdc-bridge/commit/08cbdca371d947a9ffac814b44378ffe26c475e2))

## [0.1.1](https://github.com/splattner/digitalstrom-vdc-bridge/compare/v0.1.0...v0.1.1) (2026-05-16)


### Features

* allow plugins to emit events / logs and also show them in the frontend ([#3](https://github.com/splattner/digitalstrom-vdc-bridge/issues/3)) ([6f99be8](https://github.com/splattner/digitalstrom-vdc-bridge/commit/6f99be8e0cc1eb02ba7df1c1b9950104eb8a37b0))
* enhance disovery page ([#4](https://github.com/splattner/digitalstrom-vdc-bridge/issues/4)) ([3691a74](https://github.com/splattner/digitalstrom-vdc-bridge/commit/3691a7457af435f526c8597cefa62d52729c7ec4))
* implement buttons ([#5](https://github.com/splattner/digitalstrom-vdc-bridge/issues/5)) ([b7b0c22](https://github.com/splattner/digitalstrom-vdc-bridge/commit/b7b0c22190fbda8b9c60b6fa04ede69dc6e471a4))
* implement z2m group ([#8](https://github.com/splattner/digitalstrom-vdc-bridge/issues/8)) ([fa01e1b](https://github.com/splattner/digitalstrom-vdc-bridge/commit/fa01e1b9c40742ef2f49c63ce96692d079740dd4))
* implement z2m sensor ([#7](https://github.com/splattner/digitalstrom-vdc-bridge/issues/7)) ([000ad33](https://github.com/splattner/digitalstrom-vdc-bridge/commit/000ad334704b64c3b7fa6316568cbdb12b3d3e02))
* initial implementation of the digitalstrom vDC bridge ([7138bfc](https://github.com/splattner/digitalstrom-vdc-bridge/commit/7138bfcc531084f64cab3cee8a381180b96361c5))


### Bug Fixes

* also create container images on release ([#13](https://github.com/splattner/digitalstrom-vdc-bridge/issues/13)) ([e31e0af](https://github.com/splattner/digitalstrom-vdc-bridge/commit/e31e0af6cb4efe56e96cacf58c332eb6e41d7309))
* haaddon + correct version update ([#15](https://github.com/splattner/digitalstrom-vdc-bridge/issues/15)) ([0da9ed2](https://github.com/splattner/digitalstrom-vdc-bridge/commit/0da9ed2f0c8692b6652e8d9242e42b52bd8757ef))
* set correct button primary group and allow to change ([#6](https://github.com/splattner/digitalstrom-vdc-bridge/issues/6)) ([e3d90bf](https://github.com/splattner/digitalstrom-vdc-bridge/commit/e3d90bf38838e979533eb4d6749d606068d08fb2))
* show full vdc dsuid ([#11](https://github.com/splattner/digitalstrom-vdc-bridge/issues/11)) ([2e77a31](https://github.com/splattner/digitalstrom-vdc-bridge/commit/2e77a313b4812f7032ddc803e9c1537cd31a400c))
