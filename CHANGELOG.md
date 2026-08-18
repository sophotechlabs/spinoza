# Changelog

## [1.8.1](https://github.com/sophotechlabs/spinoza/compare/v1.8.0...v1.8.1) (2026-08-18)


### Miscellaneous

* update gitignore ([d3bf588](https://github.com/sophotechlabs/spinoza/commit/d3bf58814194d373d8131a03d22765ec66d135aa))


### Build

* **release:** ship windows as a zip instead of a tarball ([0bc3d23](https://github.com/sophotechlabs/spinoza/commit/0bc3d23bd2b875c2a2fd958eba87281067e61c59))

## [1.8.0](https://github.com/sophotechlabs/spinoza/compare/v1.7.2...v1.8.0) (2026-08-18)


### Features

* **ui:** open on the cluster overview instead of an empty table ([d736f4a](https://github.com/sophotechlabs/spinoza/commit/d736f4a0069ca51cb5cfec8913e46f540db41278))


### CI

* verify the checksums and install the release the way users do ([76ad897](https://github.com/sophotechlabs/spinoza/commit/76ad897440f02516fb63b4d3090a36d2ceb66a97))

## [1.7.2](https://github.com/sophotechlabs/spinoza/compare/v1.7.1...v1.7.2) (2026-08-18)


### Bug Fixes

* **release:** list checksums under their bare filenames ([d22bb02](https://github.com/sophotechlabs/spinoza/commit/d22bb02e85e87bf7b77f935d6fde90ec3c5b9fbb))


### CI

* queue superseded runs instead of cancelling them ([9eebe5f](https://github.com/sophotechlabs/spinoza/commit/9eebe5fe2d6326328076a0d41a519f5c3423b6a0))


### Tests

* **api:** pin field types across the wire, not just their names ([41be362](https://github.com/sophotechlabs/spinoza/commit/41be362cd478aee07a34a7849655cffdb476562d))

## [1.7.1](https://github.com/sophotechlabs/spinoza/compare/v1.7.0...v1.7.1) (2026-08-18)


### Bug Fixes

* **ui:** close the top-bar menus on an outside click, escape or focus loss ([35f85bc](https://github.com/sophotechlabs/spinoza/commit/35f85bcf3d3d602bee4da90d4948139f98bcd46d))


### Miscellaneous

* update readme ([62e4e91](https://github.com/sophotechlabs/spinoza/commit/62e4e915a7eb7e5c43258570975480dd33b5ae0a))


### CI

* analyse go and the frontend with codeql ([5e1c1ad](https://github.com/sophotechlabs/spinoza/commit/5e1c1adafd502fd91d85fd91e231dd64729468c3))
* attest the release artifacts and smoke test the binary ([e8e452b](https://github.com/sophotechlabs/spinoza/commit/e8e452bfae6821e36bcab3620caf5102b599a88a))
* enable the experimental and opinionated gocritic checks ([5624fe6](https://github.com/sophotechlabs/spinoza/commit/5624fe66aa36fdc16f9ab2c59a5c894a3bc2a892))
* gate pull requests on the dependencies they add ([f03c085](https://github.com/sophotechlabs/spinoza/commit/f03c0854a58ee49343846746c4db1ae70139adc1))
* pin the commitlint version the hook downloads ([c85b8a5](https://github.com/sophotechlabs/spinoza/commit/c85b8a55e5be782a9909be50bc89807682139033))
* rescan weekly and repeat the tests nightly ([4d1715e](https://github.com/sophotechlabs/spinoza/commit/4d1715e809aa646d0ce3947f7f479475bdd6e143))
* run the integration suite against a kind cluster ([7b9f67d](https://github.com/sophotechlabs/spinoza/commit/7b9f67d8297ed6f6851d743f131a4fc1a5494102))
* stop holding a write token while the tests run ([a9d1d14](https://github.com/sophotechlabs/spinoza/commit/a9d1d14ec96c1a12753816a07215e3acdd2d848e))


### Documentation

* state what spinoza does without arguing for it ([5e77d0d](https://github.com/sophotechlabs/spinoza/commit/5e77d0d81a368a4c46f37cd5da6ddd6dcb6faad6))


### Build

* add the recipes the new ci jobs run ([c2251ad](https://github.com/sophotechlabs/spinoza/commit/c2251ad5a24cc1194e5c5e428cddf13f9a3fb2f0))
* pin kind, kubectl, helm and shellcheck ([c3d3421](https://github.com/sophotechlabs/spinoza/commit/c3d342179d8d9ccafa253c7e44d21bb1b8877081))

## [1.7.0](https://github.com/sophotechlabs/spinoza/compare/v1.6.1...v1.7.0) (2026-08-18)


### Features

* **argocd:** sync and refresh applications from the inspect panel ([d4ad6e5](https://github.com/sophotechlabs/spinoza/commit/d4ad6e5bb65ba25308f522a009e8c928496b4351))


### Bug Fixes

* **ci:** tell gh which repo to publish the release in ([82577c6](https://github.com/sophotechlabs/spinoza/commit/82577c6a8678fd74f0617efa2b407b8486a3a428))
* **desktop:** take the PATH from the login shell so credential plugins run ([4dd683a](https://github.com/sophotechlabs/spinoza/commit/4dd683a625937b76b4cb4913c12fc5434ebb5ae0))
* **ui:** say what watching every namespace costs ([d4b2a69](https://github.com/sophotechlabs/spinoza/commit/d4b2a69a23eed07058c8f1c6ad90623718d879fb))


### CI

* split release-please from the artifact build so nothing is gated on a skipped job ([93b5e04](https://github.com/sophotechlabs/spinoza/commit/93b5e049bcd0f03a032bbb90f27d5db434fa5b76))

## [1.6.1](https://github.com/sophotechlabs/spinoza/compare/v1.6.0...v1.6.1) (2026-08-18)


### CI

* publish the release only once the macos app is attached ([2734d52](https://github.com/sophotechlabs/spinoza/commit/2734d526c96f5a0f5a057516a392f0e063dacacd))
* tag the release commit before release-please scans history ([f8e839d](https://github.com/sophotechlabs/spinoza/commit/f8e839df31fd6303ee94372d3367e0a04027683c))


### Documentation

* **changelog:** replace the empty 1.5.0 entry with what it actually shipped ([11d1ca4](https://github.com/sophotechlabs/spinoza/commit/11d1ca44c4ea64662fff4c840a7c5cb2107469ab))

## [1.6.0](https://github.com/sophotechlabs/spinoza/compare/v1.5.0...v1.6.0) (2026-08-18)


### Features

* **install:** install the desktop app and say how to run both ([f37b62e](https://github.com/sophotechlabs/spinoza/commit/f37b62eefb19a498267a99e0e9ce9231d8ac1429))


### CI

* tag the draft release so the next run starts from it ([5af3282](https://github.com/sophotechlabs/spinoza/commit/5af328273d170017e655a47763354af1f89d5b63))


### Documentation

* lead with install and cut what a public reader does not need ([0c2189b](https://github.com/sophotechlabs/spinoza/commit/0c2189b06155c1f286e57697d4b099a0a3dc0a14))
* lead with the source-available terms and add screenshots ([3f65177](https://github.com/sophotechlabs/spinoza/commit/3f65177c4f2e058a65f5d1e521c97f0f6f64ff99))


### Tests

* cover the store faults, helm storage paths and remaining guards ([aae8b7d](https://github.com/sophotechlabs/spinoza/commit/aae8b7ddd0eb81ff6e6a15abb6dd0834fd5067b7))


### Build

* **desktop:** ship the macos app bundle with every release ([24eda86](https://github.com/sophotechlabs/spinoza/commit/24eda8634b843ef0333957ae7713fcdc522d6526))

## [1.5.0](https://github.com/sophotechlabs/spinoza/compare/v1.4.0...v1.5.0) (2026-08-18)

No changes. The entry generated here repeated the releases up to 1.4.0, because release-please reads the last release from the git tags and 1.4.0 was still an unpublished draft carrying no tag. Releases are tagged as soon as the draft is cut.

## [1.4.0](https://github.com/sophotechlabs/spinoza/compare/v1.3.1...v1.4.0) (2026-08-18)


### Features

* **namespace:** keep the opening namespace per cluster and only offer it on big ones ([cb0c245](https://github.com/sophotechlabs/spinoza/commit/cb0c24521e84a4ad0283750367f075a299c7bccf))
* **protect:** ask for the typed name before applying, as before deleting ([9165be5](https://github.com/sophotechlabs/spinoza/commit/9165be508859ab3dd2961b69a3159376d18fccb8))


### Bug Fixes

* **bulk:** say that the typed confirmation is the cluster name ([aca4daf](https://github.com/sophotechlabs/spinoza/commit/aca4daf93b144183b966945203262ebfb87c027b))
* **install:** report the directory the binary was installed to ([3fc3a19](https://github.com/sophotechlabs/spinoza/commit/3fc3a19d2b439fa73714049a6fb1a2e21504ed65))
* **release:** publish as draft until the artifacts are attached ([0c7ff8a](https://github.com/sophotechlabs/spinoza/commit/0c7ff8ae5bce6e754e7cca7861dd6eae6062d57b))
* **test:** make the integration tree compile and keep it that way ([47e2709](https://github.com/sophotechlabs/spinoza/commit/47e27093defeb58924d524f7d8ee539fb11957d7))


### Miscellaneous

* add codeowners ([3ff8d24](https://github.com/sophotechlabs/spinoza/commit/3ff8d240a4339f3473d690d24edcd57385bd10c2))


### CI

* ignore hashes by shape and name every workflow ([62b2f3c](https://github.com/sophotechlabs/spinoza/commit/62b2f3cde53cd00b88f35d85062a1fcc358b5007))
* teach typos about generated changelogs and kubernetes event names ([bee8843](https://github.com/sophotechlabs/spinoza/commit/bee8843e916003f8a8db26d3a467f3197d05ee0e))


### Refactoring

* **frontend:** stop exporting what nothing outside the module uses ([6f488ff](https://github.com/sophotechlabs/spinoza/commit/6f488ff04219587f0cdc5749f5b52c9ae11bf8d8))

## [1.3.1](https://github.com/sophotechlabs/spinoza/compare/v1.3.0...v1.3.1) (2026-08-18)


### Tests

* cover the batch relay, forward replacement and cache failure paths ([2c59bf1](https://github.com/sophotechlabs/spinoza/commit/2c59bf1231aeec878306e6e1c1ca50f6967542f1))
* cover the delegates, view endpoints and helper branches that had none ([e2ef58f](https://github.com/sophotechlabs/spinoza/commit/e2ef58f246573ba7111038a4d3fb22fc59dec343))

## [1.3.0](https://github.com/sophotechlabs/spinoza/compare/v1.2.0...v1.3.0) (2026-08-18)


### Features

* **events:** open on the newest 100 and load more on demand ([88482ad](https://github.com/sophotechlabs/spinoza/commit/88482ad06aa702847e758a71615a9e7ca535fb9e))
* **filter:** complete field names and values as you type ([7552dcc](https://github.com/sophotechlabs/spinoza/commit/7552dcc1374554f177f890fb583b8ebb364460e4))
* **filter:** filter the table with chips shared with the namespace picker ([1088d0c](https://github.com/sophotechlabs/spinoza/commit/1088d0c637deb2a17cfbc142a06e7fc25ebc4f9a))
* **helm:** name the namespaces a partial secret read could reach ([46eb354](https://github.com/sophotechlabs/spinoza/commit/46eb354f1f42e1ad56c75eb44a7d25d256357079))
* **panels:** let a selection reopen the collapsed details dock ([7e31da3](https://github.com/sophotechlabs/spinoza/commit/7e31da3a41a8af182f8099a2d9de7948c6911481))
* **table:** share leftover width across every column ([45eafed](https://github.com/sophotechlabs/spinoza/commit/45eafed49f43038a95172a86cb68c85a1d4b0f56))


### Bug Fixes

* **filter:** ignore a namespace chip on a kind that has none ([a890cbb](https://github.com/sophotechlabs/spinoza/commit/a890cbb380f6d431bc7db43b314b5625259041bf))
* **filter:** take the kind scope from discovery instead of the first snapshot ([adb24b9](https://github.com/sophotechlabs/spinoza/commit/adb24b9bd9878e432ffbf6268f9c8fee2ed3dfe5))
* **flux:** fill the width with the shared column rule ([57fbc6b](https://github.com/sophotechlabs/spinoza/commit/57fbc6b7e2e17ff2ffb09c3131f4a5636689e4f6))


### CI

* **release-please:** list every accepted commit type in the changelog ([647e7c7](https://github.com/sophotechlabs/spinoza/commit/647e7c7b790f4cc14f1647a08f19b28a3be5566c))


### Refactoring

* **api:** give the error envelope a named wire type ([1acba11](https://github.com/sophotechlabs/spinoza/commit/1acba11df82d16dc5092c17f7029e7a102e65a1d))
* **contexts:** share the protected-cluster confirm name ([c65848e](https://github.com/sophotechlabs/spinoza/commit/c65848eea6689354f7e698c5fe56e1bebd800429))
* **sidebar:** move the cluster Overview next to the CLUSTER group ([246f65f](https://github.com/sophotechlabs/spinoza/commit/246f65f757c3ce2cb6825851e2ea30e20a3336a5))
* **views:** derive the view type from the one list that registers views ([59a6aff](https://github.com/sophotechlabs/spinoza/commit/59a6affbce263fe5f09cdded704ab7f7353cd73b))


### Tests

* **resources:** stop the cache-sync test waiting out the default timeout ([56d8a7a](https://github.com/sophotechlabs/spinoza/commit/56d8a7adfad831b4f137f30621a0c3fc42ec26b5))

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
