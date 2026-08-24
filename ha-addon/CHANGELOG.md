# Changelog

## [0.1.16](https://github.com/splattner/digitalstrom-vdc-bridge/compare/v0.1.15...v0.1.16) (2026-08-24)


### Bug Fixes

* start bridge plugins before accepting vDC API connections ([#205](https://github.com/splattner/digitalstrom-vdc-bridge/issues/205)) ([ae2f1dc](https://github.com/splattner/digitalstrom-vdc-bridge/commit/ae2f1dc134d0871d3b1eb8d47f0feec7bf436b55))

## [0.1.15](https://github.com/splattner/digitalstrom-vdc-bridge/compare/v0.1.14...v0.1.15) (2026-08-20)


### Features

* **web:** replace browser confirm() with in-app dialog on Plugins page ([#197](https://github.com/splattner/digitalstrom-vdc-bridge/issues/197)) ([b68e362](https://github.com/splattner/digitalstrom-vdc-bridge/commit/b68e3620157441c7ea5fbcf04e413a9f123eb221))

## [0.1.14](https://github.com/splattner/digitalstrom-vdc-bridge/compare/v0.1.13...v0.1.14) (2026-08-20)


### Features

* add optional HTTP Basic Auth for standalone deployments ([#188](https://github.com/splattner/digitalstrom-vdc-bridge/issues/188)) ([feb17d0](https://github.com/splattner/digitalstrom-vdc-bridge/commit/feb17d0333d15e82ea16c344502ab7979aaf7266))
* **discovery:** push discoveryChanged events instead of polling ([#192](https://github.com/splattner/digitalstrom-vdc-bridge/issues/192)) ([97f1811](https://github.com/splattner/digitalstrom-vdc-bridge/commit/97f1811dacd033168a702d87b7c91ce211abdc71))
* recall native WLED presets for digitalSTROM scene calls ([#190](https://github.com/splattner/digitalstrom-vdc-bridge/issues/190)) ([975ee4d](https://github.com/splattner/digitalstrom-vdc-bridge/commit/975ee4dad510b08a2fbf5c339936d7081df42e68))
* **shelly:** add Shelly Gen2+ plugin (relay and dimmer control) ([#193](https://github.com/splattner/digitalstrom-vdc-bridge/issues/193)) ([5d25d1f](https://github.com/splattner/digitalstrom-vdc-bridge/commit/5d25d1f9c46c6231b87ba6b025b76722ca0295a9))
* **shelly:** bridge power metering, binary inputs, and buttons ([#195](https://github.com/splattner/digitalstrom-vdc-bridge/issues/195)) ([9e3f385](https://github.com/splattner/digitalstrom-vdc-bridge/commit/9e3f385e35b645491ff1de018c5992c610f2cb88))


### Bug Fixes

* **deps:** update module github.com/coder/websocket to v1.8.15 ([#108](https://github.com/splattner/digitalstrom-vdc-bridge/issues/108)) ([a10fdaa](https://github.com/splattner/digitalstrom-vdc-bridge/commit/a10fdaa7a25d7ca5b0a412cdf63ea2350c496fc6))
* **deps:** update module github.com/eclipse/paho.mqtt.golang to v1.5.1 ([#86](https://github.com/splattner/digitalstrom-vdc-bridge/issues/86)) ([646e53f](https://github.com/splattner/digitalstrom-vdc-bridge/commit/646e53f26a66e14b5dcc17543563da3a51d6f197))
* **deps:** update module github.com/go-chi/chi/v5 to v5.3.1 ([#130](https://github.com/splattner/digitalstrom-vdc-bridge/issues/130)) ([b84ce5d](https://github.com/splattner/digitalstrom-vdc-bridge/commit/b84ce5dcc1083ac3c2000b47fa41d0ac9b4ed5a3))
* **deps:** update module github.com/go-chi/chi/v5 to v5.3.2 ([#194](https://github.com/splattner/digitalstrom-vdc-bridge/issues/194)) ([73d7b8c](https://github.com/splattner/digitalstrom-vdc-bridge/commit/73d7b8cda63c1b807786af0f8c50a9c9b86f7062))
* **deps:** update module google.golang.org/protobuf to v1.36.12 ([#172](https://github.com/splattner/digitalstrom-vdc-bridge/issues/172)) ([707bb0f](https://github.com/splattner/digitalstrom-vdc-bridge/commit/707bb0f4aa2e37c4a4c7752767be19f62a295e5d))
* implement smooth ramp for dimChannel notifications ([#184](https://github.com/splattner/digitalstrom-vdc-bridge/issues/184)) ([677aa6c](https://github.com/splattner/digitalstrom-vdc-bridge/commit/677aa6cc1317fa70d8dea072ec66f2755248ca0b))
* remove stale --listen flag from Docker CMD ([#181](https://github.com/splattner/digitalstrom-vdc-bridge/issues/181)) ([a201a0b](https://github.com/splattner/digitalstrom-vdc-bridge/commit/a201a0b705e1007bd875318a3f5b54378bd1f28c))
* report switch-kind bridge devices as non-dimmable to the dSS ([#189](https://github.com/splattner/digitalstrom-vdc-bridge/issues/189)) ([f806780](https://github.com/splattner/digitalstrom-vdc-bridge/commit/f806780d51356c3f9b03e141ad9271ccc0c6beab))
* stop leaking plugin goroutines and add auto-retry supervision ([#191](https://github.com/splattner/digitalstrom-vdc-bridge/issues/191)) ([16173ae](https://github.com/splattner/digitalstrom-vdc-bridge/commit/16173aef12b1013640f5e41e4d0765ec59de4592))

## [0.1.13](https://github.com/splattner/digitalstrom-vdc-bridge/compare/v0.1.12...v0.1.13) (2026-05-24)


### Bug Fixes

* **deps:** update module github.com/go-chi/chi/v5 to v5.3.0 ([#83](https://github.com/splattner/digitalstrom-vdc-bridge/issues/83)) ([cf3b4d0](https://github.com/splattner/digitalstrom-vdc-bridge/commit/cf3b4d060bfb6162d054f00055450e67e275a6bd))

## [0.1.12](https://github.com/splattner/digitalstrom-vdc-bridge/compare/v0.1.11...v0.1.12) (2026-05-19)


### Features

* add mobile friendly device page ([#54](https://github.com/splattner/digitalstrom-vdc-bridge/issues/54)) ([782b42f](https://github.com/splattner/digitalstrom-vdc-bridge/commit/782b42f3157e93778d375a0685b0c75b4f510a6b))

## [0.1.11](https://github.com/splattner/digitalstrom-vdc-bridge/compare/v0.1.10...v0.1.11) (2026-05-19)


### Features

* add mobile friendly nav ([#52](https://github.com/splattner/digitalstrom-vdc-bridge/issues/52)) ([d6ebe00](https://github.com/splattner/digitalstrom-vdc-bridge/commit/d6ebe00bdc50dbdac32d3bded765c000c3dc2d7e))

## [0.1.10](https://github.com/splattner/digitalstrom-vdc-bridge/compare/v0.1.9...v0.1.10) (2026-05-18)


### Features

* activity log for bridged devices ([#45](https://github.com/splattner/digitalstrom-vdc-bridge/issues/45)) ([7a25f44](https://github.com/splattner/digitalstrom-vdc-bridge/commit/7a25f442878a39911511dd33aae3f2f221e6bd26))

## [0.1.9](https://github.com/splattner/digitalstrom-vdc-bridge/compare/v0.1.8...v0.1.9) (2026-05-17)


### Bug Fixes

* websoket connetion inside ha app not working ([#42](https://github.com/splattner/digitalstrom-vdc-bridge/issues/42)) ([622474f](https://github.com/splattner/digitalstrom-vdc-bridge/commit/622474f80aa625d24a69df1fb9372d1ea844adec))

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
