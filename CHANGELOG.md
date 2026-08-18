# Changelog

## [1.2.0](https://github.com/sophotechlabs/spinoza/compare/v1.1.0...v1.2.0) (2026-08-18)


### Features

* **flux:** show the control plane, the sync and controller usage above the resources ([f566677](https://github.com/sophotechlabs/spinoza/commit/f566677115028b560b0e5565a6690a3e8d227207))
* **ui:** one loading indicator, used by every view ([49f10a5](https://github.com/sophotechlabs/spinoza/commit/49f10a5394e1d8e584edfc35d1b3e2adf30c58d5))


### Bug Fixes

* **events:** give events their own columns and facts, and show one Event kind ([a4da2d5](https://github.com/sophotechlabs/spinoza/commit/a4da2d57c0e8c9d6a7501e5a9afa7628615fe683))
* **flux:** fill the usage bar against the limit ([a9297c5](https://github.com/sophotechlabs/spinoza/commit/a9297c513feb58d02ebc79a648049bb5d075f55b))
* **ui:** name the api group when two kinds share a name ([fccce78](https://github.com/sophotechlabs/spinoza/commit/fccce78a4effe1fd2ec31dd91441bf1b952b05a7))
* **ui:** say what the namespace offer means and keep it in the history ([1475919](https://github.com/sophotechlabs/spinoza/commit/1475919bebf0fc0d8ad283a672a2d117224d3eab))


### Performance Improvements

* **counts:** count through the metadata client and share one tally per window ([d821123](https://github.com/sophotechlabs/spinoza/commit/d82112309556e1496deb8f981c5d99efd3c48127))
* **feed:** send row changes in one frame instead of one each ([46245ca](https://github.com/sophotechlabs/spinoza/commit/46245cac7a583335c1180d4a8378a16f5575e75b))
* **helm:** list releases by metadata and decode only the newest revision ([125bf11](https://github.com/sophotechlabs/spinoza/commit/125bf11f1502c67cf526a6f1ebe686555675c383))
* **ui:** sort rows once, not twice ([8c57c8b](https://github.com/sophotechlabs/spinoza/commit/8c57c8b1b35011ae8a2c785d10a9ab6e57bf9a6a))

## [1.1.0](https://github.com/sophotechlabs/spinoza/compare/v1.0.0...v1.1.0) (2026-08-18)


### Features

* **argocd:** give Argo the graph and resource list Flux has, and separate chart from app version ([b7c62aa](https://github.com/sophotechlabs/spinoza/commit/b7c62aa0578e3ac4337c6916e67db071bb796c12))
* **argocd:** list applications from their CRDs, and tidy the top bar ([d62d1b1](https://github.com/sophotechlabs/spinoza/commit/d62d1b1ba30f95b7d99ac2accfef59698f968fbf))
* **ci:** cut GitHub releases with release-please ([0653cd4](https://github.com/sophotechlabs/spinoza/commit/0653cd4be0665842fe4a3b5b1326d10033f744d7))
* **ci:** publish coverage badges and show them on the README ([57557cb](https://github.com/sophotechlabs/spinoza/commit/57557cbb6ca0820135e3d7542922c1d091df65f0))
* **search:** find objects by name across common kinds from the palette ([ba75911](https://github.com/sophotechlabs/spinoza/commit/ba7591114d26e794745b5251548392a8e6c90886))
* **settings:** keep interface settings on the server, not in localStorage ([35cc2c2](https://github.com/sophotechlabs/spinoza/commit/35cc2c28ea2a9155751254c38a997aaec91cecf3))
* **ui:** choose the namespace a cluster opens on, and offer it once ([e799121](https://github.com/sophotechlabs/spinoza/commit/e7991216802b4e87ba42a44f14fc6ed278440b0d))
* **ui:** pick the namespace in the top bar and scope the feed to it ([203f5bb](https://github.com/sophotechlabs/spinoza/commit/203f5bb71f81cf0980fedf18d362189f99368156))
* **ui:** show every namespace by default ([d62f15b](https://github.com/sophotechlabs/spinoza/commit/d62f15b8b7b7b4cbbb5a9db181ad84cae45b624d))
* **ui:** sort nodes and pods by cpu and memory ([9c42381](https://github.com/sophotechlabs/spinoza/commit/9c423819642a561bc5b573c2766be0afbec64f17))
* **ui:** split GitOps into detected Flux and Argo CD groups, drop the status tiles ([8f36066](https://github.com/sophotechlabs/spinoza/commit/8f360663a1ab8b883535da18d346626cfc953765))
* **ui:** switch between the desktop window and a browser tab ([7383ec6](https://github.com/sophotechlabs/spinoza/commit/7383ec6208cfedac5ab05d4ef0a20fa608f88409))


### Bug Fixes

* **ci:** green the hygiene, docs and go audit jobs ([6fa0e73](https://github.com/sophotechlabs/spinoza/commit/6fa0e73e67746ac8a0e15489e1016b1312f90762))
* **frontend:** unexport helpers that nothing else imports ([c2aacdd](https://github.com/sophotechlabs/spinoza/commit/c2aacdd5c6835eae60d24237e8e273265114680c))
* **install:** match the Location header without a bracket class ([70cefc5](https://github.com/sophotechlabs/spinoza/commit/70cefc597eca8f6773cf5ff0c0c30238681d21ae))
* **ui:** open the filtered list from search, keep tooltips current, drop the native context select ([173830c](https://github.com/sophotechlabs/spinoza/commit/173830c0c7ff40c2554e597639d3e1f4259e08f8))


### Performance Improvements

* **metrics:** cache one build per window, watch node capacity, read in parallel ([4b758da](https://github.com/sophotechlabs/spinoza/commit/4b758daed410fb4856e07c43c024ca0a107ea3ee))
* **resources:** share one informer per kind and keep it warm between views ([90c0779](https://github.com/sophotechlabs/spinoza/commit/90c0779b58af07f014a5194960b8ade8286ba829))
