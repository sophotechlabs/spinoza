# Changelog

## [1.19.0](https://github.com/sophotechlabs/spinoza/compare/v1.18.0...v1.19.0) (2026-08-29)


### Features

* **checks:** 53 more checks, and your own workloads ranked above packaged ones ([1709f34](https://github.com/sophotechlabs/spinoza/commit/1709f3488f99f2a68d28e73334d46aa911f4923d))
* **checks:** decide schedulability and references against the live cluster ([8ff9681](https://github.com/sophotechlabs/spinoza/commit/8ff9681e43ce8da132956268d60966a767b15549))
* **clusters:** open several clusters as tabs, each keeping its own state ([86a379d](https://github.com/sophotechlabs/spinoza/commit/86a379dd8608f2c43b2acc9befc2df7c3f5a05b7))
* **clusters:** open, list, activate and close a cluster over the api ([cf27a65](https://github.com/sophotechlabs/spinoza/commit/cf27a65df482ac6f26187b1cdf5ece1f16788c75))
* **clusters:** remember which clusters were open and report every one's health ([5095e9e](https://github.com/sophotechlabs/spinoza/commit/5095e9e5abc5f24e10e1db4a3aa1715dd6448b51))
* **clusters:** send every request and subscription to the cluster it is for ([09220ac](https://github.com/sophotechlabs/spinoza/commit/09220aca658bc77dfdf0478ad5cc15ce3cd3b9b1))
* **clusters:** track health per cluster instead of assuming the rest are fine ([c6693f2](https://github.com/sophotechlabs/spinoza/commit/c6693f229a6584f8763b0351e8af238a14e29ab3))
* **history:** give each cluster its own writer and keep one file honest between them ([79dbb71](https://github.com/sophotechlabs/spinoza/commit/79dbb71495fcd26353f159dcc493955401e1dfbc))
* **license:** print the terms with --license and lay them down on install ([e58e66e](https://github.com/sophotechlabs/spinoza/commit/e58e66e164ab48a37b7e3455ea100ca364affe91))


### Bug Fixes

* **checks:** drop two rules a live apiserver can never trip ([60bbc9a](https://github.com/sophotechlabs/spinoza/commit/60bbc9a4b84bf39d970592635e3b7291b09af51c))
* **ci:** scope knip away from the hand-mirrored wire contract ([c7c0af9](https://github.com/sophotechlabs/spinoza/commit/c7c0af92641f17dfab0249672c283e5402bca1e8))
* **clusters:** check protection on the cluster being written to ([ff41d58](https://github.com/sophotechlabs/spinoza/commit/ff41d58b6ba992fa625308512c71c2e0cf599b8a))
* **graph:** size flow nodes and refit the canvas so edges draw ([79dee50](https://github.com/sophotechlabs/spinoza/commit/79dee509294ab8fe04f417f9c72ef48b61126e6a))
* **issues:** raise the row cap above what a real cluster produces ([68c8302](https://github.com/sophotechlabs/spinoza/commit/68c83020593eda0c90bcf8a4c3dec643125ad074))
* **mise:** pin uv so the pipx-backed tools resolve ([312b556](https://github.com/sophotechlabs/spinoza/commit/312b556dcf09a6229667871bfa88bbc7144ce6bf))
* **sidebar:** retry discovery automatically after a transient failure ([319377f](https://github.com/sophotechlabs/spinoza/commit/319377f2b30bf447152b6d9c402adab170db2ddc))
* **spinoza:** close the act-now findings from the speedrun review ([658875d](https://github.com/sophotechlabs/spinoza/commit/658875d1706be854841fea6f9b5eded1309e1c13))


### Miscellaneous

* **deps:** drop markdownlint-cli2 and the npm backend with it ([ed9b565](https://github.com/sophotechlabs/spinoza/commit/ed9b565b1a73a7e12cf5f6233e919ecc63721c85))


### Refactoring

* **clusters:** resolve the backend per request and key connections by id ([d528c74](https://github.com/sophotechlabs/spinoza/commit/d528c74e4a1f8b0b0b2f695f1e72c761446091c0))


### Tests

* **e2e:** extend e2e suite ([cf6c8cd](https://github.com/sophotechlabs/spinoza/commit/cf6c8cd43fa941e0e2ecdf8298eb129252709506))
* **install:** assert the container install lays down the copyright ([0ebc187](https://github.com/sophotechlabs/spinoza/commit/0ebc1873b0f2254f0d5dc745a67e37b5578ecb17))
* **integration:** configure kind for testing ([1dabce9](https://github.com/sophotechlabs/spinoza/commit/1dabce9329c34fecf373473565b00cb67d46825c))
* **kind:** layer the e2e and full cluster specs onto the base one ([47c754b](https://github.com/sophotechlabs/spinoza/commit/47c754b302a6a265a458ccb1e7f0476b48b515bc))
* **mcp:** drive the five write tools against a real apiserver ([5dd77c1](https://github.com/sophotechlabs/spinoza/commit/5dd77c180d496853312ba48173e6b928c906ae08))
* **server:** take the write baseline before provoking the write ([a857c39](https://github.com/sophotechlabs/spinoza/commit/a857c39b663ccc985aa351a0907d0118697b85ff))

## [1.18.0](https://github.com/sophotechlabs/spinoza/compare/v1.17.0...v1.18.0) (2026-08-29)


### Features

* **checks:** audit pods and workloads against 15 best-practice checks ([c304b84](https://github.com/sophotechlabs/spinoza/commit/c304b84dfc57693334fc778458cb861bc4a9c227))
* **checks:** page through every finding a capped group holds back ([b23369d](https://github.com/sophotechlabs/spinoza/commit/b23369d83f681ea6d81faaf20401167c3c001d9d))
* **gitops:** name the writer that took a field on a server-side applied resource ([dd0bd24](https://github.com/sophotechlabs/spinoza/commit/dd0bd24eaf39001734423234f82628a1e52437e4))
* **gitops:** the argo write surface, a per-application page, drift and diagnosis ([14a2016](https://github.com/sophotechlabs/spinoza/commit/14a20165096356ffb9b12daa45b80403e07c2e67))
* **history:** key clusters on a normalised api server url and record what spinoza did ([eea1f66](https://github.com/sophotechlabs/spinoza/commit/eea1f66ba94121ab5eb6ac93e7599d628a404ce8))
* **history:** record what spinoza changed and show it in its own view ([70baf40](https://github.com/sophotechlabs/spinoza/commit/70baf40ff8b4cf25cda981e132a29239c861b950))
* **install:** uninstall on macos and linux ([efc858f](https://github.com/sophotechlabs/spinoza/commit/efc858f66f4c2ac96cd685c410d6ca34c1fd6620))
* **install:** uninstall on windows and check build provenance on request ([31a3bf0](https://github.com/sophotechlabs/spinoza/commit/31a3bf086bc7090b6b6e828d20bef1e76b97945e))
* **issues:** rank what is broken now into a self-clearing queue ([0f2d08c](https://github.com/sophotechlabs/spinoza/commit/0f2d08cee1cdd664c3511571a861f9546021bc9d))
* **mcp:** an MCP server and command line over one cluster, in its own binary ([89d8426](https://github.com/sophotechlabs/spinoza/commit/89d842610f8c0e92fe59da581f908b496235f712))
* **packaging:** windows, the linux desktop app, deb, rpm and channel manifests ([1003450](https://github.com/sophotechlabs/spinoza/commit/1003450bcddbea86fcaacce257198c346ee8971a))
* **topology:** an ownership graph of the cluster, folded by default ([59a318c](https://github.com/sophotechlabs/spinoza/commit/59a318c4702337f59863be7b33a3e1cf04ef685b))
* **traffic:** a workload traffic graph from Cilium Hubble metrics ([ad3efd2](https://github.com/sophotechlabs/spinoza/commit/ad3efd26e2df19df574138d7e0b729dc6fc9f982))
* **update:** offer windows the powershell install command ([6a8d08e](https://github.com/sophotechlabs/spinoza/commit/6a8d08e6a1271ba190c70d3c6ccb169b3ba0e98b))


### Bug Fixes

* **a11y:** give the docks a valid role and let the keyboard reach the scrollable panes ([a7068e0](https://github.com/sophotechlabs/spinoza/commit/a7068e0e836f77ee429b78fa3311120030239293))
* **checks:** emit patches the api server accepts ([7a9b5f6](https://github.com/sophotechlabs/spinoza/commit/7a9b5f6253a3f057b28db314054f22b169f55dd4))
* **checks:** resolve owners by api group and quote what yaml would eat ([096bb05](https://github.com/sophotechlabs/spinoza/commit/096bb052d47550b2f948f64c72824f6b02d2f55c))
* **ci:** repair ci ([7fcb02c](https://github.com/sophotechlabs/spinoza/commit/7fcb02cbf54a996eaadcd0d3520b969e78a972d3))
* **gitops:** verify the terminate patch, key drift by name and stop the hidden panel polling ([1be42b9](https://github.com/sophotechlabs/spinoza/commit/1be42b93e84d80695565a09045dd78ea126daaf6))
* **install:** compute the checksum without Get-FileHash ([e91bc4d](https://github.com/sophotechlabs/spinoza/commit/e91bc4def4f7862dbce233a9b4f6b57a7bec35ac))
* **install:** stop a typed parameter swallowing the registry handle ([e04d3c8](https://github.com/sophotechlabs/spinoza/commit/e04d3c8c9e47c89a5bbdcc2c3fbbed93e06842a1))
* **issues:** bound the stall probe and stop a guess erasing a fatal row ([c325976](https://github.com/sophotechlabs/spinoza/commit/c325976b8867cede2cf5ead07d0fc561a2e4bb16))
* **issues:** read a crash loop from the terminated state, not just the waiting one ([bd9c4fa](https://github.com/sophotechlabs/spinoza/commit/bd9c4facff8f102fface1f65d292a6e27f948553))
* **mcp:** keep one bad message from ending the session and stop a slow tool blocking the rest ([db15521](https://github.com/sophotechlabs/spinoza/commit/db15521dcf1b72506311733ebaeebbd303d1de18))
* **mcp:** refuse a replica count that is not a number, and bound every tool call ([e1cc011](https://github.com/sophotechlabs/spinoza/commit/e1cc011cda321d99b3cbf2e6a27a8cfcb7c434b5))
* **overview:** count a paged event walk as one listing attempt ([f2348b2](https://github.com/sophotechlabs/spinoza/commit/f2348b27c571c455b24721fbe55a6b501b4bc711))
* **overview:** page the warning events so the newest 25 really are the newest ([022dd83](https://github.com/sophotechlabs/spinoza/commit/022dd830f389577a47762bd76c367a028d3faae6))
* **topology:** stop a test owning a shared namespace and lint fixes ([a28029f](https://github.com/sophotechlabs/spinoza/commit/a28029fe64c8375ff00c259c02b72759c99875c9))
* **traffic:** fold a crowded graph to namespaces and bound the probes ([659ef57](https://github.com/sophotechlabs/spinoza/commit/659ef5756980cff60c58e2aeb60493ec3e917e73))
* **update:** stop comparing a release against a version that does not parse ([b19be9b](https://github.com/sophotechlabs/spinoza/commit/b19be9bff66263da43d0ebe8726185b331d0bfc4))


### CI

* **hygiene:** pin editorconfig-checker through mise instead of the npm wrapper ([cdf326c](https://github.com/sophotechlabs/spinoza/commit/cdf326cceb9f343ed75510d79858311b5e705baf))
* **windows:** run the installer and its tests on arm64 as well ([10c1947](https://github.com/sophotechlabs/spinoza/commit/10c1947432292d65f3930848205d8329e93e5bae))
* **windows:** smoke the desktop build and serialise the checksum writers ([1068899](https://github.com/sophotechlabs/spinoza/commit/106889930dd443ac05de044d1b5d2b61add6d863))


### Performance

* **checks:** cap findings per group and send each object once ([9702c48](https://github.com/sophotechlabs/spinoza/commit/9702c48297eed741bde4833902aa8b3f02af16f4))
* **topology:** index pod labels so a namespace of services stops rescanning every pod ([27f18ed](https://github.com/sophotechlabs/spinoza/commit/27f18ede01364f3db865202029fc578556954120))


### Tests

* **e2e:** cover live updates, terminals, forwards, inspect and accessibility ([ccbaa62](https://github.com/sophotechlabs/spinoza/commit/ccbaa62f2f7ddcea562e42ea9ad2f836ed96c354))
* **e2e:** drive the browser UI against a kind cluster ([94cb96e](https://github.com/sophotechlabs/spinoza/commit/94cb96ea9e38913860e41264e045167e936b5226))
* **e2e:** key the table and tree assertions to the accessible names ([a6d6c63](https://github.com/sophotechlabs/spinoza/commit/a6d6c6347945cfcb48eaea935e6c78b74a0ba1fd))
* **e2e:** select rows by their name button and drop the overlapping inspect spec ([f1af148](https://github.com/sophotechlabs/spinoza/commit/f1af148dec7a43215c43b8ffd2ea54dbf85ee7a7))
* **install:** check statically for what windows powershell 5.1 lacks ([51ce068](https://github.com/sophotechlabs/spinoza/commit/51ce068b99719879663b3fa1560903a86a00ccdd))
* **install:** cover the installer with pester and verify what releases carry ([53f4e2e](https://github.com/sophotechlabs/spinoza/commit/53f4e2e48bd3e8814ebb6b13deaf03991beb39d8))
* **install:** join paths the way windows powershell 5.1 accepts ([a0e655f](https://github.com/sophotechlabs/spinoza/commit/a0e655fc75a149e2519d76eb03077ee22d383fd3))
* **install:** let a skip stand only where the platform it names is declared ([8c3c641](https://github.com/sophotechlabs/spinoza/commit/8c3c6410235c4a5a3c464b9ef4612a81dce2884e))
* **install:** run the real install path instead of mocking my own functions ([b4640e9](https://github.com/sophotechlabs/spinoza/commit/b4640e9f4dd346a15f8e957e863be8f47b62eb68))
* **issues:** make every budget and cap a test input, and cover them ([2630b80](https://github.com/sophotechlabs/spinoza/commit/2630b80af3c92edea127d1033b7a0beeddbe16a1))

## [1.17.0](https://github.com/sophotechlabs/spinoza/compare/v1.16.0...v1.17.0) (2026-08-28)


### Features

* **cronjob:** suspend, resume and run now ([4664381](https://github.com/sophotechlabs/spinoza/commit/4664381c80c3185b4cd0e51c74adb8164e3894de))
* **settings:** follow a theme another window chose ([278eaf9](https://github.com/sophotechlabs/spinoza/commit/278eaf94cbaf9fc1f896da511b4aa3c5dbfee15a))
* **tables:** sort a node metric by how much it has, not only how much it uses ([eebfad9](https://github.com/sophotechlabs/spinoza/commit/eebfad9c9d6ba285f81ae6c021759e1547008c0f))


### Bug Fixes

* **build:** give the frontend the same version as the binary ([fb808d4](https://github.com/sophotechlabs/spinoza/commit/fb808d49a9a2490bcc45f3a6c0f21598aa55f63b))
* **settings:** send only the keys this window changed ([174ab3b](https://github.com/sophotechlabs/spinoza/commit/174ab3bcbbf4a1c27aa2d8e1a5fe905ae01345ac))

## [1.16.0](https://github.com/sophotechlabs/spinoza/compare/v1.15.0...v1.16.0) (2026-08-28)


### Features

* **update:** add an update button and a switch for the automatic check ([f88b290](https://github.com/sophotechlabs/spinoza/commit/f88b2900a07874e1a19a39647e547d01670a5340))


### Bug Fixes

* **settings:** merge on write so two windows do not undo each other ([6b5bcfd](https://github.com/sophotechlabs/spinoza/commit/6b5bcfd52ffbb6c168a4787483d3e1fd444f7601))
* **update:** refuse an install script that ignores the skip ([3215aa4](https://github.com/sophotechlabs/spinoza/commit/3215aa4e6d792744b2d25808f20d7cd25cf18bdf))


### Styles

* shorten the remaining comments ([7222a20](https://github.com/sophotechlabs/spinoza/commit/7222a203962f8b30c9af6d453b311b17fe59437f))

## [1.15.0](https://github.com/sophotechlabs/spinoza/compare/v1.14.1...v1.15.0) (2026-08-28)


### Features

* **metrics:** draw a chart from what spinoza measured when there is no Prometheus ([5275939](https://github.com/sophotechlabs/spinoza/commit/52759394c723cd1dc1dc59557586e53ac592b173))
* **update:** move the release check to spinoza.tech ([344b84f](https://github.com/sophotechlabs/spinoza/commit/344b84f4bd5f9eaf7ac918584b347c67de74ccca))
* **update:** say when a newer spinoza is out and offer the command that installs it ([1b9fab5](https://github.com/sophotechlabs/spinoza/commit/1b9fab5cfa4815857d454196273860ab5d19b07c))


### Bug Fixes

* **metrics:** stop exporting a constant nothing imports ([ce288b8](https://github.com/sophotechlabs/spinoza/commit/ce288b8cef7bc6087a96673c4b6bbcc8a69f23a5))


### Styles

* cut the comments back to what the code does not say ([8118e4b](https://github.com/sophotechlabs/spinoza/commit/8118e4be9aed0e9654316c1b10f3dfd11f1767c6))
* cut the frontend comments back ([b4695ec](https://github.com/sophotechlabs/spinoza/commit/b4695ec1a27fe5f4e9fda62f0268bf6945049576))


### Refactoring

* **update:** drop the update-check flags ([8053632](https://github.com/sophotechlabs/spinoza/commit/805363299cac78405ce10ea389982c01187fc430))

## [1.14.1](https://github.com/sophotechlabs/spinoza/compare/v1.14.0...v1.14.1) (2026-08-27)


### Bug Fixes

* **columns:** read a declared column that ranges for every row, not just the first ([0b57602](https://github.com/sophotechlabs/spinoza/commit/0b5760239a329b35322d45a8e5342ac74991066f))


### Refactoring

* **logging:** stop quoting what the handler already escapes ([eaedd1f](https://github.com/sophotechlabs/spinoza/commit/eaedd1f09396d80098e5d43c9bbddb677f00b211))


### Tests

* **logs:** hold the browser's buffer against the backlog the server asks for ([7e77ac8](https://github.com/sophotechlabs/spinoza/commit/7e77ac8274bbeb3f83e219e9b57197eec33b9c46))

## [1.14.0](https://github.com/sophotechlabs/spinoza/compare/v1.13.2...v1.14.0) (2026-08-27)


### Features

* **nodes:** say how much of how much, not just a percentage ([7e5d198](https://github.com/sophotechlabs/spinoza/commit/7e5d198b36b1af80b4d1e0ebbf50cc98bcdb758c))


### Bug Fixes

* **conditions:** stop reading a node's problem detectors as readiness ([07785ae](https://github.com/sophotechlabs/spinoza/commit/07785ae05d97aa7ddb3358434e53a35e73e76df4))

## [1.13.2](https://github.com/sophotechlabs/spinoza/compare/v1.13.1...v1.13.2) (2026-08-27)


### CI

* **codeql:** scope the scan to shipped code and pin the escaping it assumes ([6b9879d](https://github.com/sophotechlabs/spinoza/commit/6b9879d5ac83b26380ab09aa5681fc5ecdc1b156))


### Tests

* **logs:** wait for the stream to notice a pod left, not for a tick to land ([d375d2e](https://github.com/sophotechlabs/spinoza/commit/d375d2ed0fe3d00d6ee4722311b86e83e2ae4806))

## [1.13.1](https://github.com/sophotechlabs/spinoza/compare/v1.13.0...v1.13.1) (2026-08-26)


### Bug Fixes

* **deps:** take echo 4.15.4 for GHSA-vfp3-v2gw-7wfq ([74d41cb](https://github.com/sophotechlabs/spinoza/commit/74d41cb7619cd3d7a58eb74bf9ba535a648184e5))


### Refactoring

* **access:** one place asks the cluster what this user may do ([52b4760](https://github.com/sophotechlabs/spinoza/commit/52b4760b01d043503ae2b05bcc578273e86fe8e8))

## [1.13.0](https://github.com/sophotechlabs/spinoza/compare/v1.12.0...v1.13.0) (2026-08-25)


### Features

* **tables:** keep declared columns current and color the ones that say if it works ([c76d617](https://github.com/sophotechlabs/spinoza/commit/c76d617078df169fa2fa066902b725c9f39b189c))
* **tables:** show custom resources the columns their own definitions ask for ([430870e](https://github.com/sophotechlabs/spinoza/commit/430870ea0042130d60f31066711de335a57aaaa6))


### Refactoring

* **server:** split the route table and handlers out of one file ([6d6e864](https://github.com/sophotechlabs/spinoza/commit/6d6e8648cf7f3fed82116f3e1e5127e35f078595))

## [1.12.0](https://github.com/sophotechlabs/spinoza/compare/v1.11.0...v1.12.0) (2026-08-21)


### Features

* **access:** tell a bulk action what the cluster will refuse, row by row ([da9289e](https://github.com/sophotechlabs/spinoza/commit/da9289e0d091eef12436a0caabc3c4dd9ba89aaa))
* **health:** flip the cluster indicator on a failed request, not on the next ping ([42b31a9](https://github.com/sophotechlabs/spinoza/commit/42b31a9f918fc189a36c057676a22328c2cb9368))
* **helm:** tell a release's buttons what the cluster will refuse ([687ef61](https://github.com/sophotechlabs/spinoza/commit/687ef615bdd53ef6bab37f71b7587c2b826604ac))
* **logs:** tail every pod of a workload in one stream ([0a8378d](https://github.com/sophotechlabs/spinoza/commit/0a8378dc27756a1b7e9efbc7f8593c60bbacc307))
* **rbac:** check gitops actions and split the log tail across pods ([f29407f](https://github.com/sophotechlabs/spinoza/commit/f29407f3f025ea7c7ad219072cdc7e16082a3c03))
* **rbac:** grey out actions the cluster would refuse ([48ba22e](https://github.com/sophotechlabs/spinoza/commit/48ba22e26bc0841d8855ec100232b618d5ecb79f))


### Bug Fixes

* **contexts:** tell every window when the cluster changes, and ask the right question before a drain ([eb56b54](https://github.com/sophotechlabs/spinoza/commit/eb56b545d6f838eb19c9bf334590d19d8de40e87))
* **kube:** tell helm and kubectl which kubeconfig spinoza was started with ([b0400eb](https://github.com/sophotechlabs/spinoza/commit/b0400ebb4a9db8a6db43ae359eba11a9655f7937))
* **ui:** say when the cluster stops answering, and refuse an apply that would overwrite blind ([59d5713](https://github.com/sophotechlabs/spinoza/commit/59d5713ff99a1a57deaeb8222a4c33875777a802))


### Miscellaneous

* configure coderabbit reviews ([e8898f2](https://github.com/sophotechlabs/spinoza/commit/e8898f279c7b1333502d656872782ca6fdaae49d))


### Refactoring

* **stores:** one atomic file write, and cover the paths that only fail on a broken socket ([67a1468](https://github.com/sophotechlabs/spinoza/commit/67a1468a12d44c61a56096c3f66ebba2a04a6f17))


### Tests

* cover the cluster ping, helm pagination, and the paths that lose work quietly ([7f7a63a](https://github.com/sophotechlabs/spinoza/commit/7f7a63ad25b2c7f6b5be82fa8ec26343bcd57157))
* **server:** wait out a feed's opening frames before breaking its socket ([7e5403e](https://github.com/sophotechlabs/spinoza/commit/7e5403e21e42846e63b99f7cee20e03a828ba01e))

## [1.11.0](https://github.com/sophotechlabs/spinoza/compare/v1.10.0...v1.11.0) (2026-08-20)


### Features

* **compare:** report drift across two contexts for a whole kind ([4c83d0a](https://github.com/sophotechlabs/spinoza/commit/4c83d0a27e34cf2a3c62dbe558b7d1359d0b6d6e))
* **events:** filter a windowed table against the whole cluster ([7ffcb87](https://github.com/sophotechlabs/spinoza/commit/7ffcb87320cf20cfed6dff674bcb9db7679adab0))
* **helm:** find and install charts from oci registries ([1a85ac2](https://github.com/sophotechlabs/spinoza/commit/1a85ac22f17e2d83be3fdcb0eb7b931b2014b7f6))
* **helm:** install a chart from a configured repository ([d5de7cf](https://github.com/sophotechlabs/spinoza/commit/d5de7cfbb6f001ae039079ac2784d4345b79166c))
* **nodeshell:** open a root shell on a node from the node panel ([878fe3b](https://github.com/sophotechlabs/spinoza/commit/878fe3b2f103df53628f9ec1b22c83115261ad86))
* **ui:** compare an object against another context ([7797e80](https://github.com/sophotechlabs/spinoza/commit/7797e80e853b30ef7a36c5f3d2c342bcdc901ba3))
* **ui:** show configmap data beside the secret values ([ae53630](https://github.com/sophotechlabs/spinoza/commit/ae5363088ebf2af69e10268a911c08a7b2265c82))
* **ui:** turn node shells on in settings, and follow the cluster a link names ([3992b15](https://github.com/sophotechlabs/spinoza/commit/3992b15050ee45a60b23274d4dd0de0edd303b44))


### Bug Fixes

* **ui:** focus a docked release on a deep link and name it in the title ([74ef4ea](https://github.com/sophotechlabs/spinoza/commit/74ef4ea54a9d2428dab780d36407255677531f84))


### Tests

* pin cluster-scoped compare and settle the column width race ([cf78f30](https://github.com/sophotechlabs/spinoza/commit/cf78f3003d8c7b26c48264c669460b055f792e49))
* pin the node shell lifecycle, chart search order and manager delegation ([8049f13](https://github.com/sophotechlabs/spinoza/commit/8049f13f7cc1a1f8432161f4060846b40e971c8a))

## [1.10.0](https://github.com/sophotechlabs/spinoza/compare/v1.9.0...v1.10.0) (2026-08-19)


### Features

* **ui:** add ui to display secrets ([c57474c](https://github.com/sophotechlabs/spinoza/commit/c57474ca3478bbab891a45144bb962a4f611a358))
* **ui:** dock the helm release detail and drill into its resources ([51c9da0](https://github.com/sophotechlabs/spinoza/commit/51c9da09d528b0aec26e62e15149729dd3371d23))


### Bug Fixes

* **ui:** colour a condition by what it means, not by its truth value ([461f884](https://github.com/sophotechlabs/spinoza/commit/461f884263609220dfd38a614b3a2082d488face))
* **ui:** keep Progressing and Initialized reading as they did ([3df1d18](https://github.com/sophotechlabs/spinoza/commit/3df1d189c9c162b08a9c101a0e45be1a6e8d7052))


### Documentation

* add helm, inspect, port-forward and drain screenshots ([bbddbe6](https://github.com/sophotechlabs/spinoza/commit/bbddbe6fa5d44ae8d9214c09fd9a38956760f46a))
* back each feature with its screenshot and restore the licence terms ([ed1fe7c](https://github.com/sophotechlabs/spinoza/commit/ed1fe7cf51a519b5cbc7bb70db3ffe3814efa1f5))
* recapture the drain plan with the new condition colours ([5134983](https://github.com/sophotechlabs/spinoza/commit/51349832ba4fb3eee857bd4ceeab93f6fa226e8f))
* rewrite the readme for a devops audience ([a990efb](https://github.com/sophotechlabs/spinoza/commit/a990efb340c58bd7e127ab7db71d1d2357b2b266))
* **server:** say why the auth cookie omits Secure ([96ac006](https://github.com/sophotechlabs/spinoza/commit/96ac006fdd4cb8ddc811d1a057313d88a4271d05))
* show the docked release detail and the resource drill-through ([2de9402](https://github.com/sophotechlabs/spinoza/commit/2de94026c4d6ae14f238e412a4d569836055a77d))

## [1.9.0](https://github.com/sophotechlabs/spinoza/compare/v1.8.1...v1.9.0) (2026-08-19)


### Features

* **ui:** say why the desktop switch is unavailable instead of hiding it ([d2af123](https://github.com/sophotechlabs/spinoza/commit/d2af12318ec14c6a0249c0bc7c2b854bc6b486bc))


### Bug Fixes

* **ci:** green sast, go lint and the integration helm upgrade test ([feaf6d1](https://github.com/sophotechlabs/spinoza/commit/feaf6d10bebd6b96c1763b4c3090274cf5b3c975))
* **cli:** rewrite the token file so mode 0600 always applies ([44acf60](https://github.com/sophotechlabs/spinoza/commit/44acf608afee877f7bf6cfb2b889acb5057dcea8))
* **helm:** block private hosts on the upgrade repo URL ([b565907](https://github.com/sophotechlabs/spinoza/commit/b565907fabd37769a9e61a7b48cf56d2b5cf134a))
* **server:** drop the run token from the address bar after load ([1005b1c](https://github.com/sophotechlabs/spinoza/commit/1005b1c0ce7300ad723010d07d53f67aa8837255))
* **server:** keep the pprof profiler off unless -pprof is set ([584a267](https://github.com/sophotechlabs/spinoza/commit/584a2678de1a81fcbe9fb9d6e562267289a6573a))
* **server:** only treat GET as a websocket upgrade ([67345d6](https://github.com/sophotechlabs/spinoza/commit/67345d6a2c08e7c1632430c760546cc78d8009e8))
* **server:** send Referrer-Policy: no-referrer on served pages ([c6ac62b](https://github.com/sophotechlabs/spinoza/commit/c6ac62b8278672cf4445a6dc8647860422b10ae3))


### CI

* attest the windows zip release artifacts ([370eabe](https://github.com/sophotechlabs/spinoza/commit/370eabe916bf2dfb482621a8c9e2db2d082326d3))


### Build

* lock the toolchain artifacts mise installs ([7364f5c](https://github.com/sophotechlabs/spinoza/commit/7364f5c793e76878ba3dad7aa95a3842caddc39d))

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
