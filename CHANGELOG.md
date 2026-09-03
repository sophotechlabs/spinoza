# Changelog

## [1.27.5](https://github.com/sophotechlabs/spinoza/compare/v1.27.4...v1.27.5) (2026-09-03)


### Bug Fixes

* **auth:** isolate entropy failure testing ([196ad61](https://github.com/sophotechlabs/spinoza/commit/196ad612384a8ba3249e53926c29519e1370dc56))
* **auth:** require POST to sign out ([0e26f05](https://github.com/sophotechlabs/spinoza/commit/0e26f05d5484ffa8218847a9ae4ad481cc0c1f79))
* **ci:** reject incomplete mutation reports ([df2c8d3](https://github.com/sophotechlabs/spinoza/commit/df2c8d335e7f7e1bbc9599e1ee0a17725ea8b501))
* **ci:** render Helm chart in vulnerability scan ([050fbd7](https://github.com/sophotechlabs/spinoza/commit/050fbd72e0b3f4ec304c9d00e643245fa5ea69e0))
* **ci:** shard mutation testing ([0130edb](https://github.com/sophotechlabs/spinoza/commit/0130edb32d99cc5e6fe209ff5d89158fb11b2d8c))
* **ci:** stabilize long-running test jobs ([433add6](https://github.com/sophotechlabs/spinoza/commit/433add6f9e452d3784430af209aeb06372704775))
* **e2e:** stabilize cluster setup and browser state ([759d09a](https://github.com/sophotechlabs/spinoza/commit/759d09ad2d0cf70bf4be91f108efcc885ef88813))
* **export:** neutralize spreadsheet formulas ([12b4662](https://github.com/sophotechlabs/spinoza/commit/12b4662ac6d003375892004116545a165f7491a6))
* **export:** satisfy switch lint ([ccfb637](https://github.com/sophotechlabs/spinoza/commit/ccfb6375b190a7745b1c6f8191dd8857d2d1ecf3))
* **frontend:** reset parameter-scoped polls ([444aa70](https://github.com/sophotechlabs/spinoza/commit/444aa706db21b5b37ef2c299f7e5dde91bbb3f28))
* **mcp:** redact GitHub tokens from logs ([c8e9b81](https://github.com/sophotechlabs/spinoza/commit/c8e9b815dc53e7ac959bc548cf263f077d9645d0))
* **test:** use valid cluster mode session secret ([39da795](https://github.com/sophotechlabs/spinoza/commit/39da795b741762e647c0d513973e2b5ef6a9838a))


### CI

* **e2e:** retry prerequisite image pulls ([c48048d](https://github.com/sophotechlabs/spinoza/commit/c48048d6bd422d0d0eb0ca09bb3add47dbb98319))
* **e2e:** split suite into focused jobs ([1ecb99a](https://github.com/sophotechlabs/spinoza/commit/1ecb99ab192c57c63f8a790a3d4fe31cde0a49f3))


### Tests

* **app:** cover startup and desktop boundaries ([6333251](https://github.com/sophotechlabs/spinoza/commit/63332512ef9579bffaaaca12e920d8be5cab2b29))
* **auth:** avoid global entropy race ([4d6967d](https://github.com/sophotechlabs/spinoza/commit/4d6967dd62aa63d31ad861ef66a5648ec6a0bf46))
* **auth:** use POST for logout ([3ce48f6](https://github.com/sophotechlabs/spinoza/commit/3ce48f66f5cc1a6d19713c1c60062e6045d0eeae))
* **ci:** ratchet mutation coverage baselines ([674a355](https://github.com/sophotechlabs/spinoza/commit/674a355603ad9e7be09c58ff38bc9ce0336e40fa))
* **e2e:** expand application journey coverage ([012c211](https://github.com/sophotechlabs/spinoza/commit/012c21178660c78381cc4e5f771d6355014a606c))
* **e2e:** expand full-cluster coverage ([9f67cca](https://github.com/sophotechlabs/spinoza/commit/9f67cca75859c5545c19e1de05d0cb4b8ffa20f9))
* **e2e:** isolate palette shortcut coverage ([4a545cd](https://github.com/sophotechlabs/spinoza/commit/4a545cde38eeab0d438d18e2ef393b313c63d59a))
* **e2e:** target the Kubernetes context picker ([2a59712](https://github.com/sophotechlabs/spinoza/commit/2a59712223d95827f7ae41c376b25fab3ac384c6))
* **e2e:** verify Argo refresh through backend state ([a5336d4](https://github.com/sophotechlabs/spinoza/commit/a5336d47b164044a6962a7d98fbff1df9ba70744))
* **e2e:** wait for resources before filter shortcut ([f2e3b43](https://github.com/sophotechlabs/spinoza/commit/f2e3b4398efae966b33ce8a0fbb35ac3822b3488))
* **frontend:** cover lazy Helm history lifecycle ([0ea1a2c](https://github.com/sophotechlabs/spinoza/commit/0ea1a2cd0a643948f0147bbfcde89197e561a3b5))
* **frontend:** satisfy event regression lint ([9f75824](https://github.com/sophotechlabs/spinoza/commit/9f75824b2adfd8c891badeb6ce9d7f08a3669ee9))
* **helm:** cover release storage boundaries ([d204385](https://github.com/sophotechlabs/spinoza/commit/d20438506d06e2acdf02589f11faa35c356677af))
* **helm:** verify paged release history ([1c38814](https://github.com/sophotechlabs/spinoza/commit/1c388141d71734f3cc73a295301990257e50a1c2))
* **mcp:** cover encoding and startup failures ([47261fc](https://github.com/sophotechlabs/spinoza/commit/47261fc2e41facb766dec90566cd3c9352413f0c))
* **traffic:** cover negative rate saturation ([70983d7](https://github.com/sophotechlabs/spinoza/commit/70983d701c10da70bd330abbe4916d5c04d864bd))

## [1.27.4](https://github.com/sophotechlabs/spinoza/compare/v1.27.3...v1.27.4) (2026-09-02)


### Bug Fixes

* **atomicfile:** reject incomplete replacements ([514e5e2](https://github.com/sophotechlabs/spinoza/commit/514e5e23cb47628af515bff2467685fa59615663))
* **ci:** rerun all release PR checks ([9f8379f](https://github.com/sophotechlabs/spinoza/commit/9f8379f428f2c5387a491dbe71f4f88ee216c944))
* **helm:** harden paginated release reads ([b9d4ad1](https://github.com/sophotechlabs/spinoza/commit/b9d4ad149d8497c607a8d983a0af8095d45823cb))
* **helm:** load release history lazily ([82bbfd4](https://github.com/sophotechlabs/spinoza/commit/82bbfd470759b1d8cb67216dd12855390c01142b))
* **helm:** reject incomplete values files ([2847cc7](https://github.com/sophotechlabs/spinoza/commit/2847cc708defd30f2f2f19f3b5f8c40da6913ab4))
* **issues:** preserve revision characters ([c1a0f56](https://github.com/sophotechlabs/spinoza/commit/c1a0f56751c10e4a92841996590066088fc49b0a))
* **issues:** use readable duration labels ([6b8d7fd](https://github.com/sophotechlabs/spinoza/commit/6b8d7fdf75f03f83120d18e9cf6118823dfad32e))
* **overview:** reject stalled warning pagination ([4fe06fa](https://github.com/sophotechlabs/spinoza/commit/4fe06fa919028163864653ff8d9f462cf056d310))
* **prom:** discard non-finite samples ([3b02491](https://github.com/sophotechlabs/spinoza/commit/3b02491df360df924dbbe549368786889cdbd3d2))
* **traffic:** keep aggregated rates finite ([1058bd0](https://github.com/sophotechlabs/spinoza/commit/1058bd095676263038d73d7d846635fbd9ec3963))


### Tests

* **auth:** cover issuer endpoint rewriting ([cb320b1](https://github.com/sophotechlabs/spinoza/commit/cb320b144ab59cf1cdf87658f18fc161f46a4a6d))
* **cluster:** cover degraded store startup ([67a6391](https://github.com/sophotechlabs/spinoza/commit/67a6391654ab8ef5642f8cb6e4efa2b033f3c007))
* **filetx:** cover inspected path open failure ([5a303aa](https://github.com/sophotechlabs/spinoza/commit/5a303aa6770d48adcd827fb35f2de253b9f37ac3))
* **kubeconfig:** cover missing working directory ([9a15d3b](https://github.com/sophotechlabs/spinoza/commit/9a15d3b3398a3518042daa77491ae8cebf2bb1b3))
* **kubeconfig:** make path failure deterministic ([48faff0](https://github.com/sophotechlabs/spinoza/commit/48faff0333c7c57bfac1eeefd898f6daa8715fa3))
* **kube:** cover partial rate limit defaults ([2e36614](https://github.com/sophotechlabs/spinoza/commit/2e36614ade6d10e57695b099e9f4b97c1ee25250))
* **server:** cover fleet cancellation boundaries ([a857870](https://github.com/sophotechlabs/spinoza/commit/a857870935750d6a5bb8d7413efd03adbd50d828))
* **toolpath:** cover rejected environment paths ([b637ec8](https://github.com/sophotechlabs/spinoza/commit/b637ec8883935ad44051e9b6abf3765a2ac3be55))
* **update:** cover installer script persistence failures ([2f8aaa7](https://github.com/sophotechlabs/spinoza/commit/2f8aaa782982884d2181f0485dfba8d6e2a6462b))

## [1.27.3](https://github.com/sophotechlabs/spinoza/compare/v1.27.2...v1.27.3) (2026-09-02)


### Bug Fixes

* **a11y:** describe cluster color swatches ([90e781a](https://github.com/sophotechlabs/spinoza/commit/90e781aa28845731df04462b54a3f279632b87d9))
* **auth:** harden OIDC local inputs ([ad07ec1](https://github.com/sophotechlabs/spinoza/commit/ad07ec1bc33fc4709b738aaf46dfbd1acb96b9ba))
* **ci:** enforce exact mutation thresholds ([e37f9fe](https://github.com/sophotechlabs/spinoza/commit/e37f9fec3433ab56c6f4e9a804b6e3f92fa423b1))
* **config:** bound local configuration reads ([c02ba3e](https://github.com/sophotechlabs/spinoza/commit/c02ba3ea784730da6c0a5139793de946815a081e))
* **filetx:** bound local file reads ([9fb138a](https://github.com/sophotechlabs/spinoza/commit/9fb138af21aa4aa7ca146530748766f6895adc31))
* **kubeconfig:** normalize default context identity ([b5f1da9](https://github.com/sophotechlabs/spinoza/commit/b5f1da9d757b9c5d1b39a9aa12f1a93aa7416694))
* **kube:** replace in-cluster config atomically ([f152a2a](https://github.com/sophotechlabs/spinoza/commit/f152a2ae2ecaa9b95e0d729ec3ad427fb9802323))
* **limits:** normalize nonpositive scan limits ([4159580](https://github.com/sophotechlabs/spinoza/commit/41595809dc671def433d2a77609b65fa1cebc910))
* **logs:** surface selector refresh failures ([e1c37c1](https://github.com/sophotechlabs/spinoza/commit/e1c37c17d92d9bb98b6fc3f97d2ee3a292565492))
* **podcount:** paginate for exact totals ([7beb419](https://github.com/sophotechlabs/spinoza/commit/7beb4196f5d77b24a2cde6232635a645a41b6a1f))
* **podcount:** reject repeated continuation tokens ([755f3c1](https://github.com/sophotechlabs/spinoza/commit/755f3c165a66374db6b3ae66d28c2938a30ef789))
* **release:** tag draft releases immediately ([9d85e06](https://github.com/sophotechlabs/spinoza/commit/9d85e0681f3f3b3e391e528e441176cbffd59f5e))
* **resources:** count paginated resources exactly ([ef0e3e2](https://github.com/sophotechlabs/spinoza/commit/ef0e3e236965976c17903025816f716763724437))
* **server:** bound and sanitize fleet reads ([376a0db](https://github.com/sophotechlabs/spinoza/commit/376a0db024da5d6875be8bdfa215e7edfe8cc62e))
* **sort:** avoid identifier overflow ([26acdec](https://github.com/sophotechlabs/spinoza/commit/26acdec7b3f5dbd08d9c09fddacc2597d64e3620))
* **state:** cancel blocked file transactions ([bb25937](https://github.com/sophotechlabs/spinoza/commit/bb25937779295b9d0d1775cbac1f8bee2680ba6d))
* **store:** serialize cross-process state updates ([2ea018f](https://github.com/sophotechlabs/spinoza/commit/2ea018f39543f3a773f746ab3373b1e5a3561867))
* **topology:** fit large graphs into view ([4d53c16](https://github.com/sophotechlabs/spinoza/commit/4d53c16484c55a1764696764f31fcba77ba41508))
* **topology:** retain mixed projected references ([ebfde87](https://github.com/sophotechlabs/spinoza/commit/ebfde87401fc2ce8972798b935b92ac5f62ff695))


### Tests

* **actions:** cover canceled queued evictions ([a20003f](https://github.com/sophotechlabs/spinoza/commit/a20003f5815ba7e3fe7c246915de79af0205ef87))
* **auth:** cover entropy and logging failures ([a0237fe](https://github.com/sophotechlabs/spinoza/commit/a0237fed60ddcc21c9daa754dfd959ab8371aebf))
* **e2e:** cover subscription recovery ([8df331a](https://github.com/sophotechlabs/spinoza/commit/8df331a7e2f1e3d10001cf72db74fdd7cf4229ea))
* **e2e:** expand access and degradation coverage ([d8e2361](https://github.com/sophotechlabs/spinoza/commit/d8e2361eba11998b1d54ba5b712bce52c3d5a2cc))
* **e2e:** expand Helm workflow coverage ([3b047d8](https://github.com/sophotechlabs/spinoza/commit/3b047d85afbed3ea064003be7dc469a22f8598d3))
* **e2e:** expand port-forward coverage ([ee10d0c](https://github.com/sophotechlabs/spinoza/commit/ee10d0c1f614594338612bf1bd3fb44d364b4c6e))
* **e2e:** expand workload log coverage ([2aa7f47](https://github.com/sophotechlabs/spinoza/commit/2aa7f47c11cdfe575fd6301af80fd4d713546a2d))
* **e2e:** stabilize cross-browser interactions ([57b2b17](https://github.com/sophotechlabs/spinoza/commit/57b2b17628d8e14baeba2b80d43481d758a4d2a5))
* **e2e:** stabilize resource table interactions ([e07d5dc](https://github.com/sophotechlabs/spinoza/commit/e07d5dc1f8896b330c1962c04589f7ce7ed480c3))
* **e2e:** target the context picker semantically ([96bc676](https://github.com/sophotechlabs/spinoza/commit/96bc676e140256274d7b49d5a5da516aed168ca9))
* **filetx:** cover locking and read boundaries ([3c590f2](https://github.com/sophotechlabs/spinoza/commit/3c590f26ce65be164271ad8c2739e5e129ceb704))
* **filetx:** cover transactional error boundaries ([76f00e3](https://github.com/sophotechlabs/spinoza/commit/76f00e3fd395be0db4f156c0178c2ab6a3a2b9ce))
* **kubeconfig:** cover persistence boundaries ([f7f784f](https://github.com/sophotechlabs/spinoza/commit/f7f784ff7f9a5eca39b01e863b9b80f3b099fb6d))
* **listerr:** cover collector failure boundaries ([d939807](https://github.com/sophotechlabs/spinoza/commit/d939807af3ba40d43a1b3afc6cc873c3a293db95))
* **logs:** cover rollout and backpressure failures ([2431a8b](https://github.com/sophotechlabs/spinoza/commit/2431a8bda18c86a85bddbc457356890202d4996c))
* **overview:** count phase fixtures from actual pods ([40f159b](https://github.com/sophotechlabs/spinoza/commit/40f159b9485dbbb9f376b53ac9cc57d2c9cc9843))
* **portforward:** cover HTTPS proxy fallback ([11eb9f2](https://github.com/sophotechlabs/spinoza/commit/11eb9f216bdf8d31f30fc8cb50e107348e1da6b7))
* **protect:** cover persistence boundaries ([462f204](https://github.com/sophotechlabs/spinoza/commit/462f204174ffb9f9848f8832b6b64a5d477bdbb5))
* **resources:** skip malformed snapshot entries ([fb14879](https://github.com/sophotechlabs/spinoza/commit/fb14879e85d343a9a6695393230da5ce2577f9b9))
* **samples:** cover runtime limit changes ([ec25a20](https://github.com/sophotechlabs/spinoza/commit/ec25a203705b8dbbf7778109c625bfcf73fb8477))
* **server:** cover canceled resource relay ([eabd6b1](https://github.com/sophotechlabs/spinoza/commit/eabd6b1fac62ee082d90440886c01ea44113153f))
* **settings:** cover persistence boundaries ([7821713](https://github.com/sophotechlabs/spinoza/commit/78217133572bb535f9ea9631e070717bec443308))
* **store:** preserve empty cell arrays ([7f0e329](https://github.com/sophotechlabs/spinoza/commit/7f0e329ae80561f2236587bb70999cdd80218376))

## [1.27.2](https://github.com/sophotechlabs/spinoza/compare/v1.27.1...v1.27.2) (2026-09-02)


### Bug Fixes

* **access:** distinguish group lists in cache keys ([6afe352](https://github.com/sophotechlabs/spinoza/commit/6afe352e94bcf4d5c5de03491d5293ccd46a2b14))
* **actions:** harden cron triggers and drain failures ([2e86cc6](https://github.com/sophotechlabs/spinoza/commit/2e86cc666e79319f864b7355656b4f6c68d597b6))
* **auth:** enforce login flow expiry ([6d4c248](https://github.com/sophotechlabs/spinoza/commit/6d4c248f5ded8c145fdc6b4ecfb5e7612d44cd66))
* **auth:** fail closed when secret generation fails ([c60f816](https://github.com/sophotechlabs/spinoza/commit/c60f8161bf7ee40dc4ed1412f226437b3710333b))
* **cluster:** cancel stalled opens on shutdown ([85d1215](https://github.com/sophotechlabs/spinoza/commit/85d12155822099de3983ac9a93f7cb7295603753))
* **clusters:** bound terminal teardown ([36d5412](https://github.com/sophotechlabs/spinoza/commit/36d54123cf36e8a12270d915b4c7226e851e0264))
* **clusters:** snapshot context protection atomically ([bef96fb](https://github.com/sophotechlabs/spinoza/commit/bef96fbcd8130f64a1ff59e0394ee545fdbd4363))
* **config:** validate cluster resource limits ([2c1aed4](https://github.com/sophotechlabs/spinoza/commit/2c1aed4423108ba6de642f1c11f780293e9e3fe3))
* **e2e:** prepare cluster-mode assets ([7e80f72](https://github.com/sophotechlabs/spinoza/commit/7e80f7211ac3e88f1d45d134b93502d353c4e764))
* **frontend:** apply namespace start to active cluster ([3925a75](https://github.com/sophotechlabs/spinoza/commit/3925a75a462fae9288d1e9b268c01ddb340dd49c))
* **frontend:** scope namespace results to active cluster ([492b32c](https://github.com/sophotechlabs/spinoza/commit/492b32c6c9a90702b3bbc117680283cb24d1b618))
* **inspect:** expose only forwardable TCP ports ([d47318d](https://github.com/sophotechlabs/spinoza/commit/d47318d8f1a55f6866245d380acfb8ce8326564e))
* **kube:** suppress warning overflow floods ([f495def](https://github.com/sophotechlabs/spinoza/commit/f495defba13478f34da48d9c9ebd53bd621a72ea))
* **listerr:** bound and deduplicate list failures ([3e625e9](https://github.com/sophotechlabs/spinoza/commit/3e625e9a6f1d032d3c645c697b46edb23fc74f41))
* **logs:** follow recreated pod generations ([3ee88ce](https://github.com/sophotechlabs/spinoza/commit/3ee88ceae44ce862c41831a820643944879bd559))
* **logs:** surface merged stream failures ([8f0f0ec](https://github.com/sophotechlabs/spinoza/commit/8f0f0ec7174659475a52d0acba37c1b385aa266c))
* **mcp:** contain handler panics and bound result limits ([5df0af8](https://github.com/sophotechlabs/spinoza/commit/5df0af84946da846b38f12002f8814f8b465128f))
* **portforward:** bound startup lifecycle and validate ports ([4e3a10b](https://github.com/sophotechlabs/spinoza/commit/4e3a10bc5d93cb4a96ca5ad4fa19747216b035ba))
* **prometheus:** bound proxy response bodies ([5ad67e9](https://github.com/sophotechlabs/spinoza/commit/5ad67e92e71ba849a09f00bd453098c75a0f808c))
* **prometheus:** use clear network alias ([8532fcd](https://github.com/sophotechlabs/spinoza/commit/8532fcd8c237ac87e1b42280cbbd82fe2de4b5d5))
* **readers:** report recovered partial failures ([54608e2](https://github.com/sophotechlabs/spinoza/commit/54608e27aba50d955782fcbc766c5b737e701a94))
* **samples:** enforce the remembered pod limit ([3a751f0](https://github.com/sophotechlabs/spinoza/commit/3a751f065522de7ffe3f6aa6529d9bc55ae6c807))
* **schema:** discard stale fetches after refresh ([6221b79](https://github.com/sophotechlabs/spinoza/commit/6221b79ed384e77830e1cb4c21d7d2ff9e05ed9d))
* **server:** harden asynchronous lifecycle handling ([4249758](https://github.com/sophotechlabs/spinoza/commit/42497584c31a3fef247321c5942caaa093d20be6))
* **toolpath:** bound login shell PATH output ([782dede](https://github.com/sophotechlabs/spinoza/commit/782dedec568a151bf712877e2465c73a57d56095))


### Tests

* **auth:** cover session sealing failures ([3831018](https://github.com/sophotechlabs/spinoza/commit/38310189234eb37d099d29f2319cdd79f4cf37be))
* **charts:** reject reserved repository addresses ([c54047d](https://github.com/sophotechlabs/spinoza/commit/c54047d27913cac140dbe13df49efb4c3ee3f389))
* **e2e:** cover traffic graph semantics ([a580b7a](https://github.com/sophotechlabs/spinoza/commit/a580b7ab6f74e1c6475ffca9d65d042eba145d80))
* **frontend:** cover stale asynchronous results ([61bdb40](https://github.com/sophotechlabs/spinoza/commit/61bdb40d2cb83fc3e7e4584f3cedda268189bd8a))
* **go:** satisfy backend lint checks ([f040a3e](https://github.com/sophotechlabs/spinoza/commit/f040a3eda281f09722944d7c97d00df9bda8739b))
* **helm:** preserve chart name ranking ([2129492](https://github.com/sophotechlabs/spinoza/commit/2129492ce52d4a7154636f5728b008422244bfa8))
* **inspect:** skip malformed ConfigMap data ([0efc84e](https://github.com/sophotechlabs/spinoza/commit/0efc84ee9bd911bc09be88d944dfa4ade1ee4bb5))
* **portforward:** cover runner lifecycle boundaries ([85c7d75](https://github.com/sophotechlabs/spinoza/commit/85c7d7557170239a18a7178868c8eccd89606222))
* **portforward:** cover start cancellation boundaries ([0e4cfec](https://github.com/sophotechlabs/spinoza/commit/0e4cfecf6d66456cfdf1a5a7478aefcafcc8210e))
* **prometheus:** cover proxy failure boundaries ([03bf359](https://github.com/sophotechlabs/spinoza/commit/03bf359a373ce26ae027fdb13fcfbfe70b4582a9))
* **resources:** cover canceled shared count waiters ([c125cdb](https://github.com/sophotechlabs/spinoza/commit/c125cdb446b36c9c205e2cb9021ce9adf1353b36))
* **resources:** register Flux list fixture ([e8f4172](https://github.com/sophotechlabs/spinoza/commit/e8f417295624f278791105fc1300056a20f0b1bb))
* **server:** cover authorization check defaults ([cb24c44](https://github.com/sophotechlabs/spinoza/commit/cb24c443ade7c627a23ddbd0da91ffacdc1ae709))
* **server:** cover closed exec writers ([5565e31](https://github.com/sophotechlabs/spinoza/commit/5565e31b6d2693a793038ab3bcb9b72afba73a91))
* **server:** cover feed failure boundaries ([c5fae4a](https://github.com/sophotechlabs/spinoza/commit/c5fae4a602e926b9cc5ccab0547156f7036890f4))
* **server:** cover fleet check close race ([14ccf1c](https://github.com/sophotechlabs/spinoza/commit/14ccf1cdcf49ac605ea9ffb5c9ae29ff057e263f))
* **server:** cover history ordering ties ([d192c59](https://github.com/sophotechlabs/spinoza/commit/d192c5969feea734b1b439348fa6b5161b246568))
* **server:** cover node shell cleanup rejection ([524d016](https://github.com/sophotechlabs/spinoza/commit/524d016d096c3ac618ad5309e9a0e19c63fa82e1))
* **server:** cover stale log relay output ([f05a435](https://github.com/sophotechlabs/spinoza/commit/f05a4358567fa569d1991793626240e05b3001f6))
* **server:** cover stale resource relay output ([5df8fde](https://github.com/sophotechlabs/spinoza/commit/5df8fdeaecfc93a58d288d8c5138217a4a685c04))
* **server:** cover timeline overload boundaries ([212d8ab](https://github.com/sophotechlabs/spinoza/commit/212d8ab1c875b4ce04b5edea6b038a4cfd40df45))
* **server:** ignore late closed cluster health ([1b68905](https://github.com/sophotechlabs/spinoza/commit/1b6890578e6fc0835d06f1e59e49dd17b9859b3f))
* **server:** keep valid live authentication ([5dbd5ea](https://github.com/sophotechlabs/spinoza/commit/5dbd5ea4ef1199be19cb119d568d68940c277c25))
* **server:** reject invalid fleet cursor encoding ([532b8fe](https://github.com/sophotechlabs/spinoza/commit/532b8fef24135bd28947b12cfe4349e05c61debe))
* **server:** stop idle health watchers ([4d9ee57](https://github.com/sophotechlabs/spinoza/commit/4d9ee57dc381013e93fbc7998fcabafeca10ae6c))
* use canonical canceled spelling ([0538875](https://github.com/sophotechlabs/spinoza/commit/05388752689fde779e4930e629d59a2f9660fa1d))

## [1.27.1](https://github.com/sophotechlabs/spinoza/compare/v1.27.0...v1.27.1) (2026-09-02)


### Bug Fixes

* **ci:** isolate main push concurrency ([17bdabf](https://github.com/sophotechlabs/spinoza/commit/17bdabf4915123801064812aca49c024bb82d5bf))
* **ci:** preserve main push validation ([e5d912c](https://github.com/sophotechlabs/spinoza/commit/e5d912c5d844d5ddf11059782b58827b5e64bfca))
* **ci:** satisfy Go lint checks ([01e1731](https://github.com/sophotechlabs/spinoza/commit/01e1731d05c340dc95c20f22d55b723dbe8b1318))
* **ci:** supersede obsolete main validation ([4b28899](https://github.com/sophotechlabs/spinoza/commit/4b28899991cafc92cffa5fca78374183fdd23767))
* **clusters:** clear stale errors on switch ([c85ab74](https://github.com/sophotechlabs/spinoza/commit/c85ab7491a764a1337a3a61fbd4c8bfaa64773e9))
* **clusters:** restart log streams ([7e43d6e](https://github.com/sophotechlabs/spinoza/commit/7e43d6ebe01a1d1356e1a7503f53dbeb77730315))
* **commandbuffer:** distinguish exact-limit output ([e5c85cc](https://github.com/sophotechlabs/spinoza/commit/e5c85cc917d9487cf6e2a188e7fe2eb0c75dea85))
* **frontend:** discard stale action completions ([13a8438](https://github.com/sophotechlabs/spinoza/commit/13a8438c975205d568a53158286d1f10b3526d07))
* **frontend:** isolate cluster-scoped state ([bc7c89b](https://github.com/sophotechlabs/spinoza/commit/bc7c89bc4b6c4ba31587a05941a3070a031b0a42))
* **frontend:** isolate report and history lifecycles ([20a33b1](https://github.com/sophotechlabs/spinoza/commit/20a33b14f6365cfae41a24ca450851fbe15b77d6))
* **gitops:** isolate application state ([dc1d279](https://github.com/sophotechlabs/spinoza/commit/dc1d279f8ed8834bc9d04b65f8f8a477ac669f92))
* **kubeconfig:** deduplicate default sources ([1a256e3](https://github.com/sophotechlabs/spinoza/commit/1a256e38b82441045a62e6251e988b30afbe2048))
* **release:** fail on draft lookup errors ([f67970e](https://github.com/sophotechlabs/spinoza/commit/f67970ef146820e928699d511dc878ebae23553b))
* **release:** scope draft detection permission ([3d2a416](https://github.com/sophotechlabs/spinoza/commit/3d2a4165952042811a61555fe4eaabdde0586df5))
* **server:** enforce local shell connection budgets ([13d158a](https://github.com/sophotechlabs/spinoza/commit/13d158abdb3cb6ff6f12129a609d7aacfae3157b))


### Tests

* **atomicfile:** cover directory sync failures ([bfaa31b](https://github.com/sophotechlabs/spinoza/commit/bfaa31ba4ed4b1191614630f379841a1a8ec0717))
* **auth:** cover OIDC failure boundaries ([27d7c17](https://github.com/sophotechlabs/spinoza/commit/27d7c1769602322ff423611574f01626dbe93dd8))
* **charts:** cover repository network boundaries ([5167342](https://github.com/sophotechlabs/spinoza/commit/5167342332937193ee2ae7253af0dcb55802c2fe))
* **charts:** cover token connection failures ([59d0b4d](https://github.com/sophotechlabs/spinoza/commit/59d0b4da6dbbf4b5ffaf7f69c18fd56e6e330a17))
* **checks:** cover referenced ConfigMap scanning ([8bad4a4](https://github.com/sophotechlabs/spinoza/commit/8bad4a4b0dae57e13f803ea2519a58e4eb235d3b))
* **cluster:** cover construction and partial discovery ([bf8bba3](https://github.com/sophotechlabs/spinoza/commit/bf8bba36f6eddd2226afd3d674efc958fe75c7a0))
* **e2e:** cover cluster source management ([8351b5a](https://github.com/sophotechlabs/spinoza/commit/8351b5a16cb060e752d35b4d9c6e610f80277ca4))
* **e2e:** cover persistent panel state ([a32ce72](https://github.com/sophotechlabs/spinoza/commit/a32ce729deb9fdcd0f8e3719fb28291b3d36a12d))
* **e2e:** cover protected cluster mutations ([92924ea](https://github.com/sophotechlabs/spinoza/commit/92924ea2dd215588c7b9089ce3ffc6d6c9b89064))
* **e2e:** cover workload actions and cancellation ([7139455](https://github.com/sophotechlabs/spinoza/commit/713945561a202ce2753c96874529b96cc9e6303a))
* **e2e:** expand audit worklist coverage ([cca4a0c](https://github.com/sophotechlabs/spinoza/commit/cca4a0c48bc92e62f85f5beb6b01a11aa25600e2))
* **e2e:** expand checks workflow coverage ([eaf3324](https://github.com/sophotechlabs/spinoza/commit/eaf3324b47d7f7cc2419415cf3d0a7cc3fe0fa42))
* **e2e:** expand history workflow coverage ([6bf7f34](https://github.com/sophotechlabs/spinoza/commit/6bf7f3467dc59dc584e35286551c86ec4e86027b))
* **e2e:** expand issue queue coverage ([294cb4c](https://github.com/sophotechlabs/spinoza/commit/294cb4c7c151ce99ecc700a663fa01a011bdc6b3))
* **e2e:** expand metrics workflow coverage ([bed65d8](https://github.com/sophotechlabs/spinoza/commit/bed65d83bbad75144ce7b67df43c0e304ff3e59a))
* **e2e:** match fleet resource labels ([e5969e0](https://github.com/sophotechlabs/spinoza/commit/e5969e0e421aa31be5156108e995e1088171256b))
* **e2e:** preload kind fixture images ([997af36](https://github.com/sophotechlabs/spinoza/commit/997af367101d5359df7df1c55b60c5884b12db9e))
* **e2e:** select rows through accessible controls ([dfde1b2](https://github.com/sophotechlabs/spinoza/commit/dfde1b20d3ce047c71cd592ddac5d126e4c5c975))
* **e2e:** verify reconnect subscriptions ([19742aa](https://github.com/sophotechlabs/spinoza/commit/19742aa017e8f0d79e52e7032a330899d55e48aa))
* **exec:** cover panic and command selection ([1913322](https://github.com/sophotechlabs/spinoza/commit/191332297e50149bea841a0442524eba6fee611e))
* **helm:** cover namespace lookup failures ([28e0878](https://github.com/sophotechlabs/spinoza/commit/28e08780cf73c42a17370f4581bc19233dbd940c))
* **helm:** cover runner and persistence failures ([fffb5a0](https://github.com/sophotechlabs/spinoza/commit/fffb5a08d177f823f0ad5e6eff7fbe4312c4e9a2))
* **helm:** cover truncated namespace fallback ([8199384](https://github.com/sophotechlabs/spinoza/commit/8199384639b87e660e6d3c5416f23befe905c221))
* **logs:** cover duplicate attachments ([6692c9a](https://github.com/sophotechlabs/spinoza/commit/6692c9a41a8eb844b87a2b5db0156c64cb2839ae))
* **logs:** cover pod list failures ([4a95700](https://github.com/sophotechlabs/spinoza/commit/4a9570034c9fbec2f3304b29b7d9465e0a061cfb))
* **mcp:** cover protocol transport failures ([9d092fd](https://github.com/sophotechlabs/spinoza/commit/9d092fd0a92dce328208314ecbbe23e8c0657b8c))
* **nodeshell:** cover cleanup boundaries ([31f6abf](https://github.com/sophotechlabs/spinoza/commit/31f6abfa887bb9d5d8f16b6b449bbed437c11525))
* **portforward:** cover reservation cleanup ([f5e0eb6](https://github.com/sophotechlabs/spinoza/commit/f5e0eb6547fb533f23af55966753d69293d7e090))
* **portforward:** cover stale run races ([d0626a8](https://github.com/sophotechlabs/spinoza/commit/d0626a8726d5326dc42663e974c0c65616b0fbcf))
* **prom:** cover history query failures ([85b85a3](https://github.com/sophotechlabs/spinoza/commit/85b85a339e57dd2e0d1021c82499ccf8df37ddf1))
* **resources:** benchmark informer cache memory ([0967617](https://github.com/sophotechlabs/spinoza/commit/0967617c7ac058b3012a22b1bb45418540753cd7))
* **resources:** cover cache edge cases ([806e04f](https://github.com/sophotechlabs/spinoza/commit/806e04f23e5dda884cee1d8a029e57892c69c23b))
* **resources:** cover cache visibility boundaries ([c2ea9f8](https://github.com/sophotechlabs/spinoza/commit/c2ea9f8bf556443369adf60919e46b7d1409c5cd))
* **resources:** cover flux owner discovery failures ([3cbe961](https://github.com/sophotechlabs/spinoza/commit/3cbe961b8859c12f764520472d3a868a546e8e8f))
* **resources:** cover GitOps graph failures ([9808b7b](https://github.com/sophotechlabs/spinoza/commit/9808b7b9e7c590134d9510996e531d3ba949afb5))
* **resources:** cover invalid printer paths ([419e8e5](https://github.com/sophotechlabs/spinoza/commit/419e8e565f706685013ee8cbd9c5070a2284dad3))
* **resources:** cover owner lookup failures ([8e55ee3](https://github.com/sophotechlabs/spinoza/commit/8e55ee3479ab2c8508d3fb65ecb81efaf52fb683))
* **resources:** cover stream lifecycle races ([e6a91b9](https://github.com/sophotechlabs/spinoza/commit/e6a91b9da37adbed1c2d35e32b8cc1f0070ad6ae))
* **resources:** cover timeline delivery boundaries ([04e5a05](https://github.com/sophotechlabs/spinoza/commit/04e5a05c719f0f02b071a9b71e6b152e01e2932c))
* **resources:** detach failed subscriptions ([2d0f746](https://github.com/sophotechlabs/spinoza/commit/2d0f7466e5a3a9979794e5134f529506d4c86332))
* **server:** cover active cluster restoration ([2f3f6f9](https://github.com/sophotechlabs/spinoza/commit/2f3f6f9910c16f94528f7dfc5e0d4e9120544f6f))
* **server:** cover API fallback boundaries ([7086f8a](https://github.com/sophotechlabs/spinoza/commit/7086f8af9171d3c50a405c46ba1f536695abfd09))
* **server:** cover auth and baseline boundaries ([12363a9](https://github.com/sophotechlabs/spinoza/commit/12363a95a5bc212ee5f0cd1a9d556c4012cbefb9))
* **server:** cover auth and fleet history boundaries ([60e2e66](https://github.com/sophotechlabs/spinoza/commit/60e2e6657df76654494735521e2ef1a905dfc190))
* **server:** cover browser switch cancellation ([33b8fd0](https://github.com/sophotechlabs/spinoza/commit/33b8fd07a87a370352de755684b6963ef35cfdea))
* **server:** cover cluster health shutdown ([172a919](https://github.com/sophotechlabs/spinoza/commit/172a919f55981ca9e35549a7886fda0c8358d7d9))
* **server:** cover concurrent shell draining ([2841d4c](https://github.com/sophotechlabs/spinoza/commit/2841d4cb27dc340ec7b8396476ffd2069a9abbd6))
* **server:** cover default live connection limits ([08951fc](https://github.com/sophotechlabs/spinoza/commit/08951fc74e4c55d3b39f8eec0f29885bffc6585f))
* **server:** cover fleet and baseline boundaries ([e824410](https://github.com/sophotechlabs/spinoza/commit/e824410d8370f9bd1f1d0fa12bf737cc7c1c4cd7))
* **server:** cover idle view timer boundaries ([5643da1](https://github.com/sophotechlabs/spinoza/commit/5643da175468f4571f5fa325046e7019602b822f))
* **server:** cover state and policy boundaries ([e83ff9b](https://github.com/sophotechlabs/spinoza/commit/e83ff9bd0b2bd2a9baeeada2b4ae2008492f08a0))
* **server:** cover timeline persistence failures ([442d56c](https://github.com/sophotechlabs/spinoza/commit/442d56c017db867d0a488ab7bfe59ef92b8135c0))
* **server:** cover unsigned settings access ([59cf059](https://github.com/sophotechlabs/spinoza/commit/59cf059c2a86b4d5205581e2500109f0cc828c09))
* **server:** limit Argo action bodies ([60e9e85](https://github.com/sophotechlabs/spinoza/commit/60e9e852583fd5387028a67c518ce6b2f35351b9))
* **server:** release refused shell upgrades ([72ba595](https://github.com/sophotechlabs/spinoza/commit/72ba595501b258706fbb61830b68331703941904))
* **server:** report local comparison failures ([6da94f1](https://github.com/sophotechlabs/spinoza/commit/6da94f199add3056e0fec404e2c01adbf4fbc84b))
* **server:** stabilize fleet namespace ordering ([5b80a0f](https://github.com/sophotechlabs/spinoza/commit/5b80a0fca857449308ee9bcd0580a32f8df3c703))
* **server:** validate fleet finding requests ([6e618a3](https://github.com/sophotechlabs/spinoza/commit/6e618a39597d5748f782befc1b2ddbb468c1f61d))
* **traffic:** verify live Hubble metrics ([4d17cf9](https://github.com/sophotechlabs/spinoza/commit/4d17cf9d4419e82c4d3acfb77f1f4307657f7f54))
* **update:** cover interrupted release responses ([f07ecf4](https://github.com/sophotechlabs/spinoza/commit/f07ecf4c9bb07f01b5d4368577d31e613385292a))

## [1.27.0](https://github.com/sophotechlabs/spinoza/compare/v1.26.1...v1.27.0) (2026-09-01)


### Bug Fixes

* **checks:** compare structured probe values ([6953c4c](https://github.com/sophotechlabs/spinoza/commit/6953c4ca544441c43c8250cc5db19a4cf8b42278))
* **checks:** make quota selection deterministic ([b4084e5](https://github.com/sophotechlabs/spinoza/commit/b4084e57a5df24b410720a47fdee0b5c41114a42))
* **ci:** validate release pull requests automatically ([62cf979](https://github.com/sophotechlabs/spinoza/commit/62cf979a76bda7754d358f6246b483e275e0d194))
* **frontend:** discard stale action and query results ([e73f8a8](https://github.com/sophotechlabs/spinoza/commit/e73f8a8c830380113d66e746d6bb520784af8735))
* **frontend:** isolate cluster-bound asynchronous state ([cb9337f](https://github.com/sophotechlabs/spinoza/commit/cb9337f22a18642e7ec6cd99ab1d54009b07e66e))
* **frontend:** scope polling errors to requests ([3d5f915](https://github.com/sophotechlabs/spinoza/commit/3d5f915e9b0d3acf8e1070610a6b0fe3eb8672e9))
* **helm:** reject oversized payloads ([2a97f05](https://github.com/sophotechlabs/spinoza/commit/2a97f0558f47680d37087b9d8a532fa13e208c5e))
* **helm:** report OCI catalog failures ([d97aec5](https://github.com/sophotechlabs/spinoza/commit/d97aec5d40b767df7017db1d7a25cbac9445f121))
* **history:** preserve timeline and persistence invariants ([1d26717](https://github.com/sophotechlabs/spinoza/commit/1d26717d3135110cc5c8c04ba769f80e8e805dad))
* **release:** make draft recovery reliable ([a74f7fb](https://github.com/sophotechlabs/spinoza/commit/a74f7fbc5a741ba8a66d8639c274e3da91033f6a))
* **release:** make validation self-contained ([a149b17](https://github.com/sophotechlabs/spinoza/commit/a149b171d9bf2293502863e0a388d5d598ad65e4))
* **release:** recognize squash commit subjects ([79b02c0](https://github.com/sophotechlabs/spinoza/commit/79b02c080ed51815475d4b5b623e9cb4e3c7b686))
* **release:** recover drafts automatically ([5c07e41](https://github.com/sophotechlabs/spinoza/commit/5c07e4150fef0f358ae3e07b9ff05c40e8766181))
* **release:** restore release boundaries ([df995dc](https://github.com/sophotechlabs/spinoza/commit/df995dc6f436042de54a30b211c829d14fa732a0))
* **release:** use native release recovery ([2157e57](https://github.com/sophotechlabs/spinoza/commit/2157e57b2031faedd7ea8a40bff8a0a4529b4a12))
* **resources:** compile root cancellation ([4109099](https://github.com/sophotechlabs/spinoza/commit/410909974b8bfeae4ce46c056f0871ecb4a24b5d))
* **runtime:** bound subprocess output ([f244e8a](https://github.com/sophotechlabs/spinoza/commit/f244e8a3218f5f5627d581ae1fc53332fcfe4440))
* **security:** enforce auth and cluster scope boundaries ([636e60b](https://github.com/sophotechlabs/spinoza/commit/636e60bdec7aeed19def719df5886f1591ecb6fd))
* **server:** strictly decode bounded JSON bodies ([eee74b9](https://github.com/sophotechlabs/spinoza/commit/eee74b92402b563dc724910e2f0a69a6e9dfc4ea))
* **settings:** serialize migration and synchronization ([0c0c4c7](https://github.com/sophotechlabs/spinoza/commit/0c0c4c7d7e1cfbe6c5ec1c06042659e43df6b4ba))
* **store:** paginate history by timestamp and ID ([7da75dd](https://github.com/sophotechlabs/spinoza/commit/7da75dddd8b4e2bc3a7395af5a6951649c52dea0))
* **update:** order rechecks and bound responses ([c880f7e](https://github.com/sophotechlabs/spinoza/commit/c880f7ea6b503256f083321f39c4c482709bcc3c))


### CI

* cancel superseded test campaigns ([0a745aa](https://github.com/sophotechlabs/spinoza/commit/0a745aa9d041ac83bb7080d7627a2ce57c213133))
* **e2e:** let main test campaigns finish ([8438bf0](https://github.com/sophotechlabs/spinoza/commit/8438bf009921ed59355b3bc90713f9fc193f757c))
* skip redundant test campaigns ([a038789](https://github.com/sophotechlabs/spinoza/commit/a0387899c18c5d93eaddceb1f748209656594f54))


### Tests

* **actions:** cover Kubernetes action boundaries ([13cbd22](https://github.com/sophotechlabs/spinoza/commit/13cbd22747afdafcbe7851b0815937166a108f7f))
* **auth:** harden identity and kube client boundaries ([ae8c089](https://github.com/sophotechlabs/spinoza/commit/ae8c089232aa90c3ceb4150416980d71acefe7b4))
* **checks:** cover malformed scheduling facts ([bd99e8e](https://github.com/sophotechlabs/spinoza/commit/bd99e8e5baf80340964a9c1a08e4f73e404f1cc9))
* **checks:** keep scanning malformed RBAC values ([609ba9b](https://github.com/sophotechlabs/spinoza/commit/609ba9bd40942df7c307dfa9267b7366e26b1d8c))
* **checks:** keep scanning supplemental groups ([ee9f9fc](https://github.com/sophotechlabs/spinoza/commit/ee9f9fc275a5472b899a96a6299e63337cae051d))
* **checks:** strengthen malformed input coverage ([4f43c03](https://github.com/sophotechlabs/spinoza/commit/4f43c0358bd8e69d355840fb18a70cebdbda5e87))
* **ci:** use typed concurrency contracts ([3be8d23](https://github.com/sophotechlabs/spinoza/commit/3be8d2333e83fcf421274ac8f5cb29690dd259c0))
* **cluster:** cover client wiring and malformed comparisons ([67b7df7](https://github.com/sophotechlabs/spinoza/commit/67b7df748897461e4fb33d6ce5d38aeaf812f51b))
* **concurrency:** cover MCP and log lifecycle limits ([ef34a5c](https://github.com/sophotechlabs/spinoza/commit/ef34a5c64d0b0ee75b0c8288045a25f7f165d10a))
* **core:** cover service boundary failures ([9ef8b10](https://github.com/sophotechlabs/spinoza/commit/9ef8b10e75c0ac151d202ef05adb50056064b69b))
* cover listening and stored settings behavior ([9aaf590](https://github.com/sophotechlabs/spinoza/commit/9aaf590b628898ce849ab43e670d74f9c13a85ec))
* **e2e:** audit major application surfaces ([323d24f](https://github.com/sophotechlabs/spinoza/commit/323d24f37f30cf7701886d23ea9a47de5dc5bf3a))
* **e2e:** cover persisted browser behavior ([fafbcc0](https://github.com/sophotechlabs/spinoza/commit/fafbcc0cdf2430b5885ed38f2fc4f7d072a70791))
* **frontend:** cover paging and migration failures ([6f2c189](https://github.com/sophotechlabs/spinoza/commit/6f2c189cad3a4dcfda1e494eab2a1b75aaf2b708))
* **frontend:** cover stale asynchronous results ([eab5d8d](https://github.com/sophotechlabs/spinoza/commit/eab5d8d560d62c91ff448b1c9f42f30f293c80f9))
* **frontend:** cover stale request failures ([8ea3e27](https://github.com/sophotechlabs/spinoza/commit/8ea3e27249b28f85ff7a6980863d36be5b3bf229))
* **frontend:** finish stale asynchronous regressions ([ce5d696](https://github.com/sophotechlabs/spinoza/commit/ce5d696baebebd2869030787cd8e29f567ece022))
* **gitops:** cover ordering and malformed state ([d4ca5d0](https://github.com/sophotechlabs/spinoza/commit/d4ca5d0d5cb5d3f61830a76e379535dcae9a25e9))
* **go:** cover concurrent lifecycle edges ([90c4d64](https://github.com/sophotechlabs/spinoza/commit/90c4d642db752ac128bf86aa8b8dc26434109253))
* **go:** harden concurrent state persistence ([d042d67](https://github.com/sophotechlabs/spinoza/commit/d042d678fa679c19dcdd8ee1419d34d9218781bd))
* **helm:** cover release cache eviction ([344c691](https://github.com/sophotechlabs/spinoza/commit/344c69154b1738799335210d0f3e45c757105de2))
* **issues:** cover malformed and boundary states ([7ed6a0a](https://github.com/sophotechlabs/spinoza/commit/7ed6a0a7d34baeb76d4eb9174291c88d5545a3cd))
* **nodeshell:** cover cleanup failure boundaries ([3482a93](https://github.com/sophotechlabs/spinoza/commit/3482a930c1faa41a2a97d1b533f9c08a21a3ce33))
* **nodeshell:** fix lint spelling ([136400a](https://github.com/sophotechlabs/spinoza/commit/136400a82c46170c9934c669f2b7731f5ee8f2d6))
* **rbac:** cover grant count ordering ([8048cbe](https://github.com/sophotechlabs/spinoza/commit/8048cbe011d78260d6908c89d5ca9ba063b4d8a4))
* **release:** align assertions with recovery workflow ([289ddd4](https://github.com/sophotechlabs/spinoza/commit/289ddd426bcf65ac3d3718e25c2e273457296a98))
* **release:** clean up workflow assertions ([c4faec6](https://github.com/sophotechlabs/spinoza/commit/c4faec6937f760393bd4148fc5c8ab83f5ade7b8))
* **resources:** cover filtering and fallback boundaries ([580aa1b](https://github.com/sophotechlabs/spinoza/commit/580aa1b18d0294135e88562ca899c9dae4997a80))
* **resources:** expand inventory and manager coverage ([667c254](https://github.com/sophotechlabs/spinoza/commit/667c254beb5469582e35b982d4ce4e7dab570a90))
* **server:** cover fleet limits and authorization mapping ([055034d](https://github.com/sophotechlabs/spinoza/commit/055034d853ffdc66a93506646c01f33e00f7b53f))
* **server:** mark health helper ([1262119](https://github.com/sophotechlabs/spinoza/commit/1262119ed919ab15afa15a9fbbcb9634a71ab7b7))
* **settings:** cover zero-value store ([740f571](https://github.com/sophotechlabs/spinoza/commit/740f571eb5fccad81d2e2d9a98e9efbc2dcb9022))
* **storage:** cover persistence and update recovery ([166ed81](https://github.com/sophotechlabs/spinoza/commit/166ed819fb786bd22ac743325ac72fd5e78d7890))
* **store:** use a valid baseline fixture ([0e0923a](https://github.com/sophotechlabs/spinoza/commit/0e0923a790dc0c19b8fae18ab47712c6df8e9aa4))
* **topology:** cover builder boundaries ([13711f1](https://github.com/sophotechlabs/spinoza/commit/13711f169ce2197cc01e9f4aba93af7959e935ec))

## [1.26.1](https://github.com/sophotechlabs/spinoza/compare/v1.26.0...v1.26.1) (2026-09-01)


### Bug Fixes

* **cluster:** restore persisted timeline ([3d29302](https://github.com/sophotechlabs/spinoza/commit/3d293026c618e1ab391f26a58652b025c27ba2e6))
* **go:** stabilize unit test CI ([28286b0](https://github.com/sophotechlabs/spinoza/commit/28286b04c580ba3f9b4941a86654a10f56fc602b))
* **release:** verify Helm chart checksums ([28e41f3](https://github.com/sophotechlabs/spinoza/commit/28e41f3d37d09d597c6ca65903641942e18254fe))
* **settings:** isolate served user preferences ([63096fb](https://github.com/sophotechlabs/spinoza/commit/63096fbd497b376b3f9b5475d357ee886e61282e))


### CI

* **e2e:** run capability groups automatically ([5e714f8](https://github.com/sophotechlabs/spinoza/commit/5e714f8801434426afeebdc2124691070ba4ca36))


### Tests

* **e2e:** add capability-grouped coverage ([5434052](https://github.com/sophotechlabs/spinoza/commit/543405225e1f979c39e8d796cd944fbdfd618bbc))
* **e2e:** make existing scenarios independently runnable ([e8e222c](https://github.com/sophotechlabs/spinoza/commit/e8e222c1523736b07138469ce63992d203b2df1b))
* **go:** expand unit reliability coverage ([1796f30](https://github.com/sophotechlabs/spinoza/commit/1796f307ce1f192a59ae56b03a9a18b6a1cf92d2))

## [1.26.0](https://github.com/sophotechlabs/spinoza/compare/v1.25.0...v1.26.0) (2026-09-01)


### Features

* **access:** scope reads to the namespaces an account may list ([c9afead](https://github.com/sophotechlabs/spinoza/commit/c9afeadca617b8e037f742797121ddc09052a133))
* **api:** carry partial check coverage, the issue total and the cluster wobble ([b5a42e6](https://github.com/sophotechlabs/spinoza/commit/b5a42e688007bf37442a0ae0c094e90cbd7f12d7))
* **api:** carry the account's role and the namespaces it reads ([ab4df95](https://github.com/sophotechlabs/spinoza/commit/ab4df95231ca7370a739e5ff2c8e087a8c3e8cf2))
* **auth:** add the identity, session and oidc package ([70e63fd](https://github.com/sophotechlabs/spinoza/commit/70e63fd450a839a737eb30158a89d961cb4113b0))
* **cli:** serve a cluster to a team behind an identity provider ([7c8c50c](https://github.com/sophotechlabs/spinoza/commit/7c8c50c9fdef1fab84a01687369822159f630159))
* **frontend:** sign in and show what the account reaches ([58dd4b8](https://github.com/sophotechlabs/spinoza/commit/58dd4b8fee7a45486f05e5ea57a582c67a83f68f))
* **kube:** impersonate the signed-in account against the apiserver ([859e6f8](https://github.com/sophotechlabs/spinoza/commit/859e6f8cb410e7f7af5c0f207b661ff363763372))
* **server:** gate every route on role, locality and cluster reach ([914c4b8](https://github.com/sophotechlabs/spinoza/commit/914c4b85fb10172b6fab161ae4579961d47ca2d6))


### Bug Fixes

* **access:** fail closed on unanswered reviews ([cd6d546](https://github.com/sophotechlabs/spinoza/commit/cd6d546f2e9825e741a0919d066e7bc8973d694a))
* **atomicfile:** sync directory after replacement ([16e6951](https://github.com/sophotechlabs/spinoza/commit/16e6951cf285b108709dc36596871a784680ae86))
* **auth:** enforce session maximum age ([d68c0ed](https://github.com/sophotechlabs/spinoza/commit/d68c0edef50e1f05bae001eca8ffe74a8d0c154e))
* **auth:** enforce trusted proxy boundary ([1999a81](https://github.com/sophotechlabs/spinoza/commit/1999a817ef23e90aedf69cbad3d253261f2038c6))
* **auth:** reject weak session secrets ([f8615d6](https://github.com/sophotechlabs/spinoza/commit/f8615d6b3e41e77f5932a25c1eb6813cde262798))
* **auth:** restrict shared audit metadata ([ae28dbf](https://github.com/sophotechlabs/spinoza/commit/ae28dbf60ecf1a0ed40a9eef73e850d6fe7f39b6))
* **auth:** validate trusted proxy requests ([9d1f001](https://github.com/sophotechlabs/spinoza/commit/9d1f0016f028ce7af79eccfe1d256cc64760a666))
* **charts:** require modern TLS ([83f80f6](https://github.com/sophotechlabs/spinoza/commit/83f80f67dab7843397e6204e057fae14265b8269))
* **checks:** align namespace counts ([86a8d35](https://github.com/sophotechlabs/spinoza/commit/86a8d35ffa99d593b6cdc84d38b84f35965abed7))
* **checks:** key surveys by metrics state ([63129fe](https://github.com/sophotechlabs/spinoza/commit/63129feb75cb4c58432c2913b2abd0164f284328))
* **checks:** read API removals from the k8s.io/api markers, keyed by kind ([798f329](https://github.com/sophotechlabs/spinoza/commit/798f329638a3ab6ac59cd77ac2eb9cb6d0ca9b55))
* **checks:** reject non-boolean user rules ([47a0ff9](https://github.com/sophotechlabs/spinoza/commit/47a0ff9885915415188b4397a338c983693fd573))
* **checks:** reject unreachable silencer rules ([4acfb19](https://github.com/sophotechlabs/spinoza/commit/4acfb19e3fd3121aea0e3751da1f2d964b294d27))
* **checks:** report silencer evaluation errors ([2c54bd5](https://github.com/sophotechlabs/spinoza/commit/2c54bd513f210a3598100e4aeeb99a554f86092a))
* **ci:** name uv where a Python tool needs it, and yq for the scope guard ([e2d8c23](https://github.com/sophotechlabs/spinoza/commit/e2d8c23d4ee255e98568f1dbe7c9f787d71ed6d9))
* **ci:** prepare assets before vulnerability scans ([572d2a9](https://github.com/sophotechlabs/spinoza/commit/572d2a90c32d7dca3ffb802fac351c490299b8ac))
* **ci:** satisfy Go lint checks ([82ee33d](https://github.com/sophotechlabs/spinoza/commit/82ee33dfd38c1d5450c1e244bfc2d4ab185fe5b8))
* **ci:** scope every mise install, and stop counting a cut list as the whole ([f1393fd](https://github.com/sophotechlabs/spinoza/commit/f1393fdf32d71b89bdbf155a2e1435c6c3309c19))
* **cluster:** require public URL origin ([ac44f0d](https://github.com/sophotechlabs/spinoza/commit/ac44f0d174bb1c37bc74cc274faf9d10dd3a507a))
* **compare:** drop the spec fields each cluster allocates for itself ([1949ae9](https://github.com/sophotechlabs/spinoza/commit/1949ae9620cda99b06786e747fc13ed0a96f6497))
* **fleet:** page merged check findings ([b887a51](https://github.com/sophotechlabs/spinoza/commit/b887a513eaca1cafacd79181db6923aaa399cb63))
* **fleet:** preserve namespace provenance ([ac5688c](https://github.com/sophotechlabs/spinoza/commit/ac5688c8cafa67f44f2775efa820df8154fb0772))
* **fleet:** report partial image inventory ([c745893](https://github.com/sophotechlabs/spinoza/commit/c74589361b903ca3028b6398f8ac76ae7050f2c4))
* **frontend:** explain absent GitOps controllers ([8d001f0](https://github.com/sophotechlabs/spinoza/commit/8d001f0c32c9b58db95b1493fbacc24160cbbf70))
* **frontend:** migrate custom check rules ([d7a0971](https://github.com/sophotechlabs/spinoza/commit/d7a0971c9f4d1963ced37bc8ee02d52225d36a65))
* **frontend:** replace deprecated type matcher ([68f873a](https://github.com/sophotechlabs/spinoza/commit/68f873aab32ab3009ae06c638313f3ed54bc1808))
* **frontend:** show fleet warning ([b26644f](https://github.com/sophotechlabs/spinoza/commit/b26644f8052f3138ce004145754b92aa61b05de4))
* **health:** settle cluster verdict under one lock ([94a1bff](https://github.com/sophotechlabs/spinoza/commit/94a1bffed0a3f65f8b392558a75d46d9a3da0c5b))
* **helm:** constrain repository network access ([6a8ef2e](https://github.com/sophotechlabs/spinoza/commit/6a8ef2e0384fe0960d88b218cfb45e69e0d34394))
* **helm:** reuse cached repository indexes ([bb02a38](https://github.com/sophotechlabs/spinoza/commit/bb02a38dcfd1de6ad3d0c435aa04e6cbe0739f65))
* **history:** persist action actors ([d7081bc](https://github.com/sophotechlabs/spinoza/commit/d7081bc2486c4048082298f5b35eba886242e758))
* **history:** record action actors ([01a22df](https://github.com/sophotechlabs/spinoza/commit/01a22df162872834942c2699a4f1b5c7a93857e0))
* **history:** record partial drains as failed ([8de3f25](https://github.com/sophotechlabs/spinoza/commit/8de3f25d68b25f7387b746642eca2d751e752d56))
* **issues:** validate page cursors ([221d6a6](https://github.com/sophotechlabs/spinoza/commit/221d6a618bc4ddd0bbe7dcb883f4afc4bf6a9c58))
* **mcp:** recheck cluster protection ([73cea28](https://github.com/sophotechlabs/spinoza/commit/73cea2838c1e61586fbe467a18b7bdde878f470b))
* **release:** publish image and Helm chart ([ff8dd47](https://github.com/sophotechlabs/spinoza/commit/ff8dd47d3ab17ce3c0de0312bd824b6ea20d1e75))
* **release:** verify release tags before creation ([8466f48](https://github.com/sophotechlabs/spinoza/commit/8466f483bda9b7d04cee146eeb6e2d608e9cee28))
* **server:** avoid websocket shutdown delays ([602bbf6](https://github.com/sophotechlabs/spinoza/commit/602bbf6e7b8141d61bfcac59ce7fa17d1586fec4))
* **server:** bound and revalidate live connections ([b69ec1f](https://github.com/sophotechlabs/spinoza/commit/b69ec1f11e1794f74c3e416a788db5fb8980e5d9))
* **server:** count cluster label characters ([9f76dd3](https://github.com/sophotechlabs/spinoza/commit/9f76dd365f979e0e0bdefcc18ff754b269626244))
* **server:** finish started writes and gate the node shell before it upgrades ([e1d5d5b](https://github.com/sophotechlabs/spinoza/commit/e1d5d5b3d032dfb7e769c1255a8e5bfc3a61ed5e))
* **server:** limit websocket subscriptions ([189c894](https://github.com/sophotechlabs/spinoza/commit/189c8940795fb6c8560bb4b681246385a6ab5234))
* **server:** minimize public ([520f936](https://github.com/sophotechlabs/spinoza/commit/520f936f173f5a32db8642b53a259cc0cfe01e3d))
* **server:** release unresponsive websocket feeds ([dab5d76](https://github.com/sophotechlabs/spinoza/commit/dab5d760d276850e08d8fea8ca1fdd821ee65682))
* **streams:** synchronize subscription access ([33ea429](https://github.com/sophotechlabs/spinoza/commit/33ea4297fd258a351d1a826acbf9cb0c708e1f8f))
* **timeline:** resume saved recordings after restart ([902da76](https://github.com/sophotechlabs/spinoza/commit/902da76db657e127bca7b4de8b6ecb05b8973935))
* **update:** reject oversized install scripts ([f911b5f](https://github.com/sophotechlabs/spinoza/commit/f911b5f48f79ac5c4e4c6c10f474dedfe472e958))


### Miscellaneous

* **security:** guard OpenPGP scanner exceptions ([ec534d0](https://github.com/sophotechlabs/spinoza/commit/ec534d045adfd136d0947c4bd69a0d04b44fa132))


### CI

* cancel superseded validation runs ([00598d9](https://github.com/sophotechlabs/spinoza/commit/00598d9d22de703e6e461772b08b3f00da796057))


### Refactoring

* **auth:** consolidate signed cookie handling ([881357c](https://github.com/sophotechlabs/spinoza/commit/881357c1b85ba477edf08f6af05a7d59d70b87ef))
* **auth:** share signed cookie codec ([c375678](https://github.com/sophotechlabs/spinoza/commit/c37567854e923898217d415c553449e026ac2d2b))
* **frontend:** share view labels ([6a093ab](https://github.com/sophotechlabs/spinoza/commit/6a093ab7569e53ca7b764518adb941dbb1f51f8c))
* **server:** consolidate cluster key lookup ([50b06f6](https://github.com/sophotechlabs/spinoza/commit/50b06f6759c9fd0f1c4fe1803c0e05ceb0f950c0))
* **server:** one door for cluster writes ([b9e964e](https://github.com/sophotechlabs/spinoza/commit/b9e964e370a1121b4c8cadb8098d8d7220ec7e81))


### Tests

* **access:** cover ([6b89662](https://github.com/sophotechlabs/spinoza/commit/6b8966211772d11baedac4dbb46d46edc9113d26))
* **api:** refresh history wire contract ([9346daa](https://github.com/sophotechlabs/spinoza/commit/9346daa2766825a1430b7ffdaeb4ea750054d223))
* **api:** support branded wire types ([844f82c](https://github.com/sophotechlabs/spinoza/commit/844f82c7d04e5d1cf2faaf886c1e58830ebda755))
* **api:** update health wire contract ([4656104](https://github.com/sophotechlabs/spinoza/commit/46561045fb8e989caa11f524d672fb55d80c147a))
* **auth:** cover custom CA trust ([537709d](https://github.com/sophotechlabs/spinoza/commit/537709db5e61fd4fe909cc3e3a5aafa93c8175cc))
* **baseline:** cover legacy fingerprint data ([5565a6a](https://github.com/sophotechlabs/spinoza/commit/5565a6a81d4bcb3b3ea566327b9324638f672f62))
* **charts:** allow deliberate TLS 1.0 fixture ([ac34385](https://github.com/sophotechlabs/spinoza/commit/ac343850e753f4fe8a498d2933f9e632513c5143))
* **cluster:** cover dynamic client wiring ([04e1e1c](https://github.com/sophotechlabs/spinoza/commit/04e1e1c66d40a6d81937460eba0a9593c274f914))
* **clustermode:** verify sso and impersonation against kind and keycloak ([dfeb3ba](https://github.com/sophotechlabs/spinoza/commit/dfeb3ba42eb0582c390ba10a9645c3acb624a983))
* **counts:** cover capped failing totals ([7bd9399](https://github.com/sophotechlabs/spinoza/commit/7bd9399ddad4ef4a11127edf29a9eee9821db2e8))
* **frontend:** cover polling scope resets ([8cb1a2b](https://github.com/sophotechlabs/spinoza/commit/8cb1a2b5dded7e3888f99b89e396a075dc497226))
* **integration:** configure chart repository ([2685f65](https://github.com/sophotechlabs/spinoza/commit/2685f65b63e127606bf183c61104408ea0a7672b))
* **resources:** configure delegated chart repository ([4f3951c](https://github.com/sophotechlabs/spinoza/commit/4f3951cd63e7d812fa422ad9f8d8cfec749eda9a))
* **resources:** cover unanswered reviews ([98267c2](https://github.com/sophotechlabs/spinoza/commit/98267c2e1152328ec901073714b5e411aabf6da2))
* **server:** cover local kubeconfig selection ([9b2b631](https://github.com/sophotechlabs/spinoza/commit/9b2b6310e1bb0ff335d114965fd3d08eba2cc2e8))
* **server:** opt into anonymous cluster access ([aa2bed4](https://github.com/sophotechlabs/spinoza/commit/aa2bed4da79f7ec333d39d4b5695811efe1d74e8))
* **server:** satisfy Go lint ([6c33481](https://github.com/sophotechlabs/spinoza/commit/6c334816c006c740c5c529887a45c742c2467e66))


### Build

* **helm:** ship the chart and image, and lint the chart in the gate ([197c712](https://github.com/sophotechlabs/spinoza/commit/197c71273f96cdff21c0735002c619d694e4ff22))

## [1.25.0](https://github.com/sophotechlabs/spinoza/compare/v1.24.1...v1.25.0) (2026-08-31)


### Features

* **desktop:** write the app log to a file ([8e6e314](https://github.com/sophotechlabs/spinoza/commit/8e6e31481bf5d929f42a85ed2c9ddc2958d59660))


### Bug Fixes

* **checks:** decide orphans from the kinds that carry references, and filter on severity ([717e46a](https://github.com/sophotechlabs/spinoza/commit/717e46a9f550ef581d6902354939c6b3e3beb517))
* **cluster:** keep the context you asked for active, and settle cluster health with one writer ([901b21b](https://github.com/sophotechlabs/spinoza/commit/901b21b20be43cb32b3692656472f0e50f5df5ad))
* **counts:** page the unhealthy pod count instead of stopping at 500 ([51a776f](https://github.com/sophotechlabs/spinoza/commit/51a776f404b5f1347735290c740ddbd5f8ea4f13))
* **fleet:** keep the baseline and the skip mark, and survive a panicking cluster ([5f4866c](https://github.com/sophotechlabs/spinoza/commit/5f4866c3cd472eb8a573acf553c991a5ef411e50))
* **frontend:** key cluster health, page history per table, and gate issue cursors on their own sort ([64f44fd](https://github.com/sophotechlabs/spinoza/commit/64f44fdb135ceefa1140a547a348592a3248b64c))
* **frontend:** stop exporting queueKey ([1e95927](https://github.com/sophotechlabs/spinoza/commit/1e959273baa9d3940f301185fce8f97c87d04076))
* **rbac:** stop a rule tied to named objects answering for every object ([6677679](https://github.com/sophotechlabs/spinoza/commit/667767918331a1ea78463e0d1cafb58e4fe338ba))
* **server:** refuse every cluster write on a protected cluster and finish the ones already started ([7aed69e](https://github.com/sophotechlabs/spinoza/commit/7aed69ed7dae627886d4b2fc83e8e0d1b32e0916))


### CI

* install only the tools each release job runs ([e0fae60](https://github.com/sophotechlabs/spinoza/commit/e0fae60177acadd34e6416649f5561e26894611c))


### Refactoring

* **desktop:** lift logging, local shell and file picker out of runDesktop ([56ca8dc](https://github.com/sophotechlabs/spinoza/commit/56ca8dc7cc42f0937eb372e73121ba45a728defc))


### Tests

* e2e design ([c982322](https://github.com/sophotechlabs/spinoza/commit/c9823223c6a6fb7ea406a8376116e83eb8e7d34d))
* match every truncation wording in the scale counter filter ([6c7568a](https://github.com/sophotechlabs/spinoza/commit/6c7568a6c7123e7520a6b95f6b08cce3843e3a64))

## [1.24.1](https://github.com/sophotechlabs/spinoza/compare/v1.24.0...v1.24.1) (2026-08-31)


### Bug Fixes

* **e2e:** stop the harness killing a server it does not own ([1ff746f](https://github.com/sophotechlabs/spinoza/commit/1ff746f500ec576df4de868e586c67ab0fc92b58))
* **test:** wait for the quota controller before auditing the namespace it fills ([e94d045](https://github.com/sophotechlabs/spinoza/commit/e94d045553964aa539958dfe4f349703f3116494))


### Documentation

* carry the social preview card in the repo ([a65fdfc](https://github.com/sophotechlabs/spinoza/commit/a65fdfc23b1b87f27a45ab0f6eefdf5e7314e621))


### Tests

* **integration:** wait for the quota controller, and lint the package ([7649076](https://github.com/sophotechlabs/spinoza/commit/764907691e73727aedf0b5f5d4111af14ec3a4b2))

## [1.24.0](https://github.com/sophotechlabs/spinoza/compare/v1.23.0...v1.24.0) (2026-08-31)


### Features

* **cli:** open on a named view and context, and keep injected values inside their quotes ([472f06b](https://github.com/sophotechlabs/spinoza/commit/472f06be169ae2fc9c604a8480f0e5279da26685))


### Bug Fixes

* **clusters:** remember tab and columns on switch ([1808c8f](https://github.com/sophotechlabs/spinoza/commit/1808c8f40e73524d7462d3bd2c14ea7afc7f2f24))


### Miscellaneous

* cleanup comments ([74d916f](https://github.com/sophotechlabs/spinoza/commit/74d916f89158e9a2353eb30616e7cf280adce173))


### Refactoring

* **resources:** give each interface the manager implements its own file ([187f187](https://github.com/sophotechlabs/spinoza/commit/187f1872cddd6decb5c7c1d848efe4bd1326dc55))


### Tests

* **resources:** cover the delegations the split showed nobody was calling ([40429a9](https://github.com/sophotechlabs/spinoza/commit/40429a90305ca04cd2f0542bb666cd1da00fc9d7))

## [1.23.0](https://github.com/sophotechlabs/spinoza/compare/v1.22.0...v1.23.0) (2026-08-30)


### Features

* **checks:** compare a baseline from another cluster by count, not by finding ([db7ae52](https://github.com/sophotechlabs/spinoza/commit/db7ae52abf86a189b32caa25440e8c4917dbef5f))
* **checks:** let a rule of your own quieten a check, and carry a baseline between clusters ([709be32](https://github.com/sophotechlabs/spinoza/commit/709be324a9070c38c5055de4a7fb014c6f92b709))
* **fleet:** a fleet view, and reach older history from the timeline ([3a06124](https://github.com/sophotechlabs/spinoza/commit/3a06124e3d1ba4bd650de7005f8fa6f153f5a544))
* **fleet:** reach the release and delivery lists from the fleet view ([b8aea75](https://github.com/sophotechlabs/spinoza/commit/b8aea752ed342c50d7a4a2b01ec71eef534ac502))
* **issues:** sort by date, page the audit half, and calm the traffic graph and the cluster dot ([12cb34a](https://github.com/sophotechlabs/spinoza/commit/12cb34a60da78e1b66695d224bee8580a658b804))
* **rbac:** answer who can do what on a cluster, not only what I can ([89f6bfb](https://github.com/sophotechlabs/spinoza/commit/89f6bfb0322b524b0432d3998d53fba80befff21))
* **resources:** add columns of your own from any field, label or annotation ([5933880](https://github.com/sophotechlabs/spinoza/commit/5933880679c777efd26c80bdb35660c13765d3c7))
* **store:** bound the audit the way the timeline is bounded ([4e71403](https://github.com/sophotechlabs/spinoza/commit/4e7140333a65efcec788c67b6dc8bc7556efb104))


### Bug Fixes

* **checks:** drop the paging hook nothing calls, and a name typos rejects ([ce0b349](https://github.com/sophotechlabs/spinoza/commit/ce0b349c5a73370e8743dc7f3584cf28640093a3))
* **sidebar:** say the pods tally counts phases, not readiness ([df8d083](https://github.com/sophotechlabs/spinoza/commit/df8d0838d5a5055c7bfd4af5885c88e29a64f1bc))


### Documentation

* **readme:** the checks, clusters, issues, topology and history it never mentioned ([743a72e](https://github.com/sophotechlabs/spinoza/commit/743a72e32ec2733bce76a06c542b9429a7c49299))


### Performance

* **checks:** let a page reuse the survey the report just built ([0774f32](https://github.com/sophotechlabs/spinoza/commit/0774f32a4d402dae006984f0b24da03ebbe5b812))


### Tests

* **checks:** hold every registered check to leaving the cache alone ([b65be21](https://github.com/sophotechlabs/spinoza/commit/b65be216c2e4ab2e03a248a3942810e96886dc82))
* **e2e:** hold a view from the harness, and prove the protected apply gate ([e95007e](https://github.com/sophotechlabs/spinoza/commit/e95007e35bb339b8ed81929fdc2a3cf0ad02aa56))
* **panels:** pin the dock tablist to owning tabs and nothing else ([04c80d5](https://github.com/sophotechlabs/spinoza/commit/04c80d5b93e99d32dcf989208885ae6404d63b14))
* **protect:** hold every spelling a real file carries across the rewrite ([cd6bd28](https://github.com/sophotechlabs/spinoza/commit/cd6bd283dbfbcfd44472e2ca801a2882e7a90670))
* **store:** time the insert the write path waits on ([652c1ec](https://github.com/sophotechlabs/spinoza/commit/652c1ec3695c40c202b9aca24fefcdc720fe81df))

## [1.22.0](https://github.com/sophotechlabs/spinoza/compare/v1.21.0...v1.22.0) (2026-08-30)


### Features

* **checks:** make the findings a work list — mute, a baseline to compare against, and per-namespace ([7d3bdbd](https://github.com/sophotechlabs/spinoza/commit/7d3bdbd4790d0e575aa3ec4f06c579b666e72d86))
* **checks:** rank each finding by how far the problem actually reaches ([0dd4370](https://github.com/sophotechlabs/spinoza/commit/0dd4370a5f94e96397cf11a2fd9198ccf61f528b))
* **checks:** read the custom resources, and only when a check needs them ([bcd2b92](https://github.com/sophotechlabs/spinoza/commit/bcd2b924079d302fe1af5ba77e9c8a489fd46880))
* **checks:** tell a leftover from something a controller reads ([5753086](https://github.com/sophotechlabs/spinoza/commit/5753086969b272b37b6ffdad80f9668a0baf61af))
* **fleet:** checks, search, releases and GitOps across every open cluster ([4b815eb](https://github.com/sophotechlabs/spinoza/commit/4b815eb0e19f15e22b59dc175fa379403c70eef9))
* **issues:** hand the queue out a page at a time, keyed on the sort order ([cd5fc9f](https://github.com/sophotechlabs/spinoza/commit/cd5fc9f2f143e29c369e0d9ae8cbee887354fc0a))
* **just:** filter every test suite by name and by path ([c77f7ef](https://github.com/sophotechlabs/spinoza/commit/c77f7ef392d70bd019d0e7661af2698649cd404c))
* **spinoza:** one Issues queue across every open cluster, and a timeline of what the cluster does ([a6a0a73](https://github.com/sophotechlabs/spinoza/commit/a6a0a73e35b818c2daec59973a33ea92ef99c989))


### Bug Fixes

* **ci:** break the checks import cycle, and stop exporting what nothing imports ([fa00b3e](https://github.com/sophotechlabs/spinoza/commit/fa00b3ec8d1c34334ea7b71b18c9e5ff1a4ed684))
* **clusters:** key the namespace answer on the cluster, and let the strip scroll ([1e88a6a](https://github.com/sophotechlabs/spinoza/commit/1e88a6afb3c63fe862012bc0581fb1e772464a19))
* e2e test ([5fc0e34](https://github.com/sophotechlabs/spinoza/commit/5fc0e34bc4a8b72960c9d7a2b0c8639300f59e6e))
* **frontend:** check what production actually reaches with knip ([903b09a](https://github.com/sophotechlabs/spinoza/commit/903b09a2fcadebcab3da73b3ff4fc5ef74d250c2))
* **lint:** check a marshal, convert two identical structs, and stop shadowing net/url ([f0f7abf](https://github.com/sophotechlabs/spinoza/commit/f0f7abf9a655a47dec49f2a06e17b2884c4510c3))
* **themes:** warn when an imported theme would make the editor unreadable ([9187c45](https://github.com/sophotechlabs/spinoza/commit/9187c45e7db46b4bb823814bf37e8ec4a3d700a0))


### Miscellaneous

* lint fix ([92634f3](https://github.com/sophotechlabs/spinoza/commit/92634f3e3601ddfada104fced3d09eb890c020fd))


### CI

* update e2e setup ([bc18f76](https://github.com/sophotechlabs/spinoza/commit/bc18f76bea139e6eb4fdd09f36e215d9f4564a76))


### Refactoring

* **frontend:** put one graph shell behind the gitops and traffic canvases ([afd26ff](https://github.com/sophotechlabs/spinoza/commit/afd26ff6d7987ff7b6f8d0c9404b2cbfbca4cb66))
* **server:** answer helm, traffic and shell support in one capability probe ([ebf88ea](https://github.com/sophotechlabs/spinoza/commit/ebf88eaff9abf6bfffdb2f80a5b3a3882ab6ddda))


### Performance

* **overview:** stop walking the cluster to count each pod phase ([a0ac93d](https://github.com/sophotechlabs/spinoza/commit/a0ac93d67bfb60e8b9af04e25e20da840d0c13d5))
* **resources:** let a one-off List release its informer ([20ddc37](https://github.com/sophotechlabs/spinoza/commit/20ddc37219f7ed7f296be1c3c0b11453f2736860))


### Tests

* **e2e:** assert the history copy the app actually renders ([3ddaa99](https://github.com/sophotechlabs/spinoza/commit/3ddaa99f357b9b4d88a9eede802ba83a3790492a))
* **e2e:** assert the upgrade renders its manifest before applying it ([3f0d537](https://github.com/sophotechlabs/spinoza/commit/3f0d5372b4525e59cd986a641fd5864e760f7dfe))
* **e2e:** open a second live cluster and switch between them ([e9c8cd7](https://github.com/sophotechlabs/spinoza/commit/e9c8cd7037d8392600bcf7a0879de6b53e2db82f))
* **e2e:** upgrade a release, install from a repo, resize a shell, and open a second cluster ([6cc3632](https://github.com/sophotechlabs/spinoza/commit/6cc3632364ae8f3367c677b2665ac18967063992))
* **history:** cover every read and write that fails ([932f8aa](https://github.com/sophotechlabs/spinoza/commit/932f8aad54d08a0fd8358c785b020fe0fb0005c1))
* **mcp:** put cmd/spinoza-mcp inside every Go gate ([2c81e3e](https://github.com/sophotechlabs/spinoza/commit/2c81e3e7705bad8cb661cabe5f4573b4f4078207))
* **shots:** capture the site's screenshots against a seeded two-cluster setup ([96ee074](https://github.com/sophotechlabs/spinoza/commit/96ee074b62ae352028dbbb56d0b7203e48d41eea))
* **shots:** capture the site's screenshots, and wait for dialogs instead of sampling ([70800eb](https://github.com/sophotechlabs/spinoza/commit/70800eb6e3e26b70d39e0661cea923a4a5aefaf4))

## [1.21.0](https://github.com/sophotechlabs/spinoza/compare/v1.20.0...v1.21.0) (2026-08-29)


### Features

* **checks:** warn about APIs a release removes, watch certificates ([bb2e7d0](https://github.com/sophotechlabs/spinoza/commit/bb2e7d0464414128915676c9adff60ef3d1f7e3d))
* **checks:** write own checks as CEL expressions ([d183aa8](https://github.com/sophotechlabs/spinoza/commit/d183aa8829fecc9040b22875a3b9a8127a000b48))
* **clusters:** give every tab a colour and name the cluster before anything destructive ([e1f6c4e](https://github.com/sophotechlabs/spinoza/commit/e1f6c4e766fba09aa63e609eb7f61459355ccec1))
* **clusters:** name a tab, group it, and choose whether it comes back ([425d1b8](https://github.com/sophotechlabs/spinoza/commit/425d1b8696c66811c30d5a2ed112dd0d9b1a068a))


### Bug Fixes

* **clusters:** give the cluster spinoza starts on a tab of its own ([ba93a77](https://github.com/sophotechlabs/spinoza/commit/ba93a778c690d06af3bdb283250830665871d1c1))
* **e2e:** lint fixes ([ba4b627](https://github.com/sophotechlabs/spinoza/commit/ba4b627055d43c6b5ec2249de2291b15d00e8b46))
* **lint:** scope the a11y relaxations to the three files that need them ([cf8b442](https://github.com/sophotechlabs/spinoza/commit/cf8b44295f8cb588cb31ffa78e155634659c63fb))
* **prom:** refuse a range query that matched more than one series ([a0f616d](https://github.com/sophotechlabs/spinoza/commit/a0f616d8a507e6f0442ca8859dcba731086280c3))
* **traffic:** read how many flow series carry labels, not whether any do ([b627fb3](https://github.com/sophotechlabs/spinoza/commit/b627fb3a582b19264ebffc15420e44484549a777))


### Miscellaneous

* **frontend:** stop exporting a checks key only its own file uses ([62b8c94](https://github.com/sophotechlabs/spinoza/commit/62b8c947ba8e4ae8402827f3aeab7a098328a313))
* **frontend:** stop exporting two helpers only their own file uses ([0acf655](https://github.com/sophotechlabs/spinoza/commit/0acf6550136d766576ee29de0dc008bcd6000905))


### CI

* **e2e:** run the browser suite on forgejo ([d825d23](https://github.com/sophotechlabs/spinoza/commit/d825d2382cd6105027eed3c51c90e5ad3e09d55b))
* **hooks:** catch unused exports before the push instead of in CI ([f5da3fe](https://github.com/sophotechlabs/spinoza/commit/f5da3feb4a148f39ecc341328075c69f66870ac6))


### Performance

* **podcount:** keep the probe page a filtered count already paid for ([2ee7033](https://github.com/sophotechlabs/spinoza/commit/2ee703357f2bea00f1292814b6864ff95926a6cc))


### Tests

* **e2e:** drive flux, argo and a cluster at scale, and the paths behind a refusal ([129aaab](https://github.com/sophotechlabs/spinoza/commit/129aaab2b3f5e8f3197439075dba9678425fb8f5))
* **e2e:** give the suite a real chart repository to read ([a7d5067](https://github.com/sophotechlabs/spinoza/commit/a7d506760de4f439447a3bcf29fc841d79fb20b8))
* **e2e:** hand the harness the cluster name the recipe made ([1b1e890](https://github.com/sophotechlabs/spinoza/commit/1b1e89054fae1ae0dd44b8626d6503e369fa0ba6))
* **panels:** pin the tab strip semantics every docked view renders through ([f6fe12b](https://github.com/sophotechlabs/spinoza/commit/f6fe12bc9ec4d48a944f7317e2c0bd548c98d444))

## [1.20.0](https://github.com/sophotechlabs/spinoza/compare/v1.19.0...v1.20.0) (2026-08-29)


### Features

* **checks:** audit RBAC, networking and storage, and choose what the audit reads and shows ([935d363](https://github.com/sophotechlabs/spinoza/commit/935d36366c59fc3dbdec68c7bd9051838e805387))


### Bug Fixes

* **ci:** read the whole archive listing so verify-archives stops failing the release ([8f4088f](https://github.com/sophotechlabs/spinoza/commit/8f4088fd839279dd7e380cd680b8e1e4eddc0be8))
* **themes:** paint the editor from the theme instead of leaving monaco its own greys ([18e7d66](https://github.com/sophotechlabs/spinoza/commit/18e7d66550e44c161d2a7fd91bd640dc7a9ef134))


### CI

* **e2e:** run the browser suite on both cluster tiers ([ad38789](https://github.com/sophotechlabs/spinoza/commit/ad387890de8eff56448e828631df04da3bad3121))


### Tests

* **e2e:** drive the editor, the write actions and the degradation paths in a browser ([ad9c989](https://github.com/sophotechlabs/spinoza/commit/ad9c989fd5069bf7847fa947ac2c3b114c0eda8b))
* **e2e:** roll back and uninstall a release, and carry the context in every view link ([983b629](https://github.com/sophotechlabs/spinoza/commit/983b6292fbd359139bec58b7d54191e58cf9a5de))
* **themes:** check every shipped theme, and the editor colours, for contrast ([63feab2](https://github.com/sophotechlabs/spinoza/commit/63feab27d4c8e5369f8f7bb691dd6a2a20f5b7b6))

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
