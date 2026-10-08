# Corpus results

Every row is one public module at one pinned version, audited with no
configuration. The index is 0 to 100 and lower is better. The two points columns
hold the points that each dimension contributed to the index. Production counts
cover the measured production set only. A run of make corpus rewrites this file.

| module | version | index | complexity-erosion points | duplication points | production code lines | production functions | eroded functions | clone groups | completeness |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| github.com/mattn/go-isatty | v0.0.20 | 17 | 17 | 0 | 142 | 15 | 1 | 0 | complete |
| github.com/pkg/errors | v0.9.1 | 0 | 0 | 0 | 277 | 32 | 0 | 0 | complete |
| github.com/fatih/color | v1.17.0 | 13 | 0 | 13 | 392 | 83 | 0 | 3 | complete |
| github.com/google/uuid | v1.6.0 | 25 | 19 | 6 | 805 | 72 | 3 | 1 | complete |
| github.com/go-chi/chi/v5 | v5.1.0 | 19 | 15 | 4 | 2655 | 234 | 5 | 2 | complete |
| github.com/gorilla/websocket | v1.5.3 | 31 | 31 | 0 | 2938 | 165 | 10 | 0 | complete |
| github.com/stretchr/testify | v1.9.0 | 46 | 28 | 18 | 3180 | 205 | 10 | 11 | complete |
| go.uber.org/zap | v1.27.0 | 32 | 20 | 12 | 4897 | 663 | 7 | 10 | complete |
| github.com/rs/zerolog | v1.33.0 | 51 | 28 | 23 | 5611 | 568 | 16 | 28 | complete |
| gopkg.in/yaml.v3 | v3.0.1 | 64 | 47 | 17 | 7710 | 302 | 61 | 22 | complete |
| github.com/prometheus/client_golang | v1.19.1 | 46 | 27 | 19 | 9330 | 657 | 23 | 28 | complete |
| github.com/valyala/fasthttp | v1.55.0 | 55 | 36 | 19 | 14392 | 1172 | 56 | 35 | complete |
| k8s.io/apimachinery | v0.31.0 | 52 | 33 | 19 | 29518 | 2435 | 91 | 72 | complete |
| google.golang.org/grpc | v1.65.0 | 52 | 35 | 17 | 50191 | 3644 | 137 | 87 | complete |
| github.com/prometheus/prometheus | v0.54.0 | 66 | 42 | 24 | 75973 | 4660 | 285 | 256 | complete |

Over the 7 modules above 5000 production lines, the Spearman rank correlation of the index is +0.45 with production code lines and +0.47 with debt per thousand production lines. Debt is eroded functions plus clone groups.
